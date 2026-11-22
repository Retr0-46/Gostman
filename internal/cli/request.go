package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"gostman/internal/collection"
)

func buildRequest(cmd *cobra.Command, url string, options *sendOptions) (*collection.Request, error) {
	method := strings.ToUpper(strings.TrimSpace(options.method))
	if method == "" {
		return nil, fmt.Errorf("method must not be empty")
	}

	headers := make(http.Header)
	for _, header := range options.headers {
		name, value, ok := strings.Cut(header, ":")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid header %q: expected 'Name: value'", header)
		}
		headers.Add(name, strings.TrimSpace(value))
	}
	if len(options.form) > 0 && headers.Get("Content-Type") != "" {
		return nil, fmt.Errorf("Content-Type is generated for --form; remove the Content-Type header")
	}
	form, err := readFormParts(options.form)
	if err != nil {
		return nil, err
	}

	body := []byte(options.data)
	if cmd.Flags().Changed("data-file") {
		var err error
		if options.dataFile == "-" {
			body, err = io.ReadAll(cmd.InOrStdin())
		} else {
			body, err = os.ReadFile(options.dataFile)
		}
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
	}

	return &collection.Request{
		Method:  method,
		URL:     url,
		Headers: headers,
		Body:    body,
		Form:    form,
	}, nil
}
