package cli

import (
	"bufio"
	"fmt"
	"io"
	"sort"

	"gostman/internal/collection"
	"gostman/internal/expect"
	"gostman/internal/httpclient"
)

func printResponse(out io.Writer, req *collection.Request, resp *httpclient.Response) error {
	w := bufio.NewWriter(out)
	fmt.Fprintf(w, "Request: %s %s\n", req.Method, req.URL)
	fmt.Fprintf(w, "Final URL: %s\n", resp.URL)
	fmt.Fprintf(w, "Status: %s\n", resp.Status)
	fmt.Fprintf(w, "Protocol: %s\n", resp.Proto)
	fmt.Fprintf(w, "Duration: %s\n", resp.Duration)
	fmt.Fprintf(w, "Body size: %d bytes\n\nHeaders:\n", len(resp.Body))

	keys := make([]string, 0, len(resp.Headers))
	for key := range resp.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range resp.Headers[key] {
			fmt.Fprintf(w, "%s: %s\n", key, value)
		}
	}

	fmt.Fprint(w, "\nBody:\n")
	w.Write(resp.Body)
	if len(resp.Body) == 0 || resp.Body[len(resp.Body)-1] != '\n' {
		fmt.Fprintln(w)
	}
	return w.Flush()
}

func printChecks(out io.Writer, results []expect.Result) error {
	w := bufio.NewWriter(out)
	fmt.Fprintln(w, "Checks:")
	for _, result := range results {
		if result.Passed {
			fmt.Fprintf(w, "  PASS %s: %s\n", result.Target, result.Actual)
		} else {
			fmt.Fprintf(w, "  FAIL %s: expected %s, got %s\n", result.Target, result.Expected, result.Actual)
		}
	}
	return w.Flush()
}
