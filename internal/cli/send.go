package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gostman/internal/collection"
	"gostman/internal/expect"
	"gostman/internal/httpclient"
)

var ErrRunFailed = errors.New("request run failed")

type sendOptions struct {
	method       string
	data         string
	dataFile     string
	form         []string
	headers      []string
	matches      []string
	expects      []string
	expectStatus []string
	expectJSON   []string
	timeout      time.Duration
	rps          int
	clients      int
	duration     time.Duration
}

func sendCmd() *cobra.Command {
	options := &sendOptions{}
	cmd := &cobra.Command{
		Use:   "send <url>",
		Short: "Send an HTTP request and show the response",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSend(cmd, args[0], options)
		},
	}
	cmd.Flags().StringVarP(&options.method, "method", "X", "GET", "HTTP method")
	cmd.Flags().StringArrayVarP(&options.headers, "header", "H", nil, "Request header in 'Name: value' format (repeatable)")
	cmd.Flags().StringVarP(&options.data, "data", "d", "", "Request body as text")
	cmd.Flags().StringVar(&options.dataFile, "data-file", "", "Read request body from a file, or - for stdin")
	cmd.Flags().StringArrayVarP(&options.form, "form", "F", nil, "Multipart field 'name=value' or file 'name=@path' (repeatable)")
	cmd.Flags().StringArrayVar(&options.matches, "match", nil, "Replace {name} with values, e.g. '{id}=(1, 2, 3)' (repeatable)")
	cmd.Flags().StringArrayVar(&options.expects, "expect", nil, "Check status, header, or JSON, e.g. 'status=(200, 404)' (repeatable)")
	cmd.Flags().StringArrayVar(&options.expectStatus, "expect-status", nil, "Expected HTTP status, e.g. 201 or '{code}' (repeatable)")
	cmd.Flags().StringArrayVar(&options.expectJSON, "expect-json", nil, "Expected JSON field, e.g. 'id=123' or '/user/id=123' (repeatable)")
	cmd.Flags().DurationVar(&options.timeout, "timeout", 30*time.Second, "Request timeout")
	cmd.Flags().IntVar(&options.rps, "rps", 0, "Total requests per second in load mode (up to 100)")
	cmd.Flags().IntVarP(&options.clients, "clients", "c", 1, "Concurrent clients in load mode (up to 20)")
	cmd.Flags().DurationVar(&options.duration, "duration", 10*time.Second, "Load test duration (up to 30s)")
	cmd.MarkFlagsMutuallyExclusive("data", "data-file", "form")
	return cmd
}

func runSend(cmd *cobra.Command, url string, options *sendOptions) error {
	if options.timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	expectations, err := options.expectationSpecs()
	if err != nil {
		return err
	}
	req, err := buildRequest(cmd, url, options)
	if err != nil {
		return err
	}
	cases, err := collection.Expand(req, options.matches, expectations...)
	if err != nil {
		return err
	}
	suite, err := expect.Compile(expectations, cases)
	if err != nil {
		return err
	}
	if cmd.Flags().Changed("rps") {
		return runLoad(cmd, cases, suite, options, len(expectations) > 0)
	}
	if cmd.Flags().Changed("clients") || cmd.Flags().Changed("duration") {
		return fmt.Errorf("--clients and --duration require --rps")
	}

	client := httpclient.NewWithTimeout(options.timeout)
	out := cmd.OutOrStdout()
	requestErrors, passedChecks, failedChecks := 0, 0, 0
	for i, testCase := range cases {
		if len(options.matches) > 0 {
			if _, err := fmt.Fprintf(out, "Case %d/%d: %s\n", i+1, len(cases), testCase.Label); err != nil {
				return err
			}
		}
		resp, err := client.Do(cmd.Context(), testCase.Request)
		if err != nil {
			requestErrors++
			if _, writeErr := fmt.Fprintf(out, "Error: %v\n", err); writeErr != nil {
				return writeErr
			}
		} else {
			if err := printResponse(out, testCase.Request, resp); err != nil {
				return err
			}
			if len(expectations) > 0 {
				results := suite.Check(i, resp)
				if err := printChecks(out, results); err != nil {
					return err
				}
				for _, result := range results {
					if result.Passed {
						passedChecks++
					} else {
						failedChecks++
					}
				}
			}
		}
		if len(options.matches) > 0 && i < len(cases)-1 {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	if len(options.matches) > 0 || len(expectations) > 0 {
		if _, err := fmt.Fprintf(out, "\nCompleted: requests=%d, errors=%d\n", len(cases), requestErrors); err != nil {
			return err
		}
	}
	if len(expectations) > 0 {
		if _, err := fmt.Fprintf(out, "Checks: %d passed, %d failed\n", passedChecks, failedChecks); err != nil {
			return err
		}
	}
	if requestErrors > 0 || failedChecks > 0 {
		return fmt.Errorf("%w: %d request errors, %d failed checks", ErrRunFailed, requestErrors, failedChecks)
	}
	return nil
}

func (options *sendOptions) expectationSpecs() ([]string, error) {
	specs := append([]string(nil), options.expects...)
	for _, value := range options.expectStatus {
		specs = append(specs, "status="+value)
	}
	for _, value := range options.expectJSON {
		path, expected, ok := strings.Cut(value, "=")
		if !ok || path == "" {
			return nil, fmt.Errorf("invalid --expect-json %q: expected 'id=123' or '/user/id=123'", value)
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		specs = append(specs, "json:"+path+"="+expected)
	}
	return specs, nil
}
