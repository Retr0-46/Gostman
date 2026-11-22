package cli

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSendMultipartFileAndFields(t *testing.T) {
	file := filepath.Join(t.TempDir(), "payload.bin")
	data := []byte{0, 255, '{', 'x', '}'}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
			t.Errorf("Content-Type = %q, error = %v", r.Header.Get("Content-Type"), err)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("multipart reader: %v", err)
			return
		}
		var names, values []string
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("next part: %v", err)
				return
			}
			payload, err := io.ReadAll(part)
			if err != nil {
				t.Errorf("read part: %v", err)
				return
			}
			names = append(names, part.FormName())
			if part.FileName() != "" {
				if part.FileName() != "payload.bin" || part.Header.Get("Content-Type") != "application/octet-stream" || !bytes.Equal(payload, data) {
					t.Errorf("file part: filename=%q, type=%q, data=%v", part.FileName(), part.Header.Get("Content-Type"), payload)
				}
			} else {
				values = append(values, string(payload))
			}
		}
		if strings.Join(names, ",") != "label,file,label,at" || strings.Join(values, ",") != "first,second,@literal" {
			t.Errorf("names=%q, values=%q", names, values)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL, "-X", "POST", "-F", "label=first", "-F", "file=@"+file, "-F", "label=second", "-F", "at=@@literal", "--expect-status", "201")
	if err != nil || !strings.Contains(out, "PASS status: 201") {
		t.Errorf("error=%v, output:\n%s", err, out)
	}
}

func TestSendMultipartMatchPreservesFileBytes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "payload.bin")
	data := []byte("binary {name} data")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}
		if r.FormValue("name") != strings.TrimPrefix(r.URL.Path, "/") {
			t.Errorf("name=%q, path=%q", r.FormValue("name"), r.URL.Path)
		}
		filePart, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("form file: %v", err)
			return
		}
		defer filePart.Close()
		got, _ := io.ReadAll(filePart)
		if !bytes.Equal(got, data) {
			t.Errorf("file data = %q, want %q", got, data)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	out, err := executeSend(nil, "send", server.URL+"/{name}", "-X", "POST", "-F", "name={name}", "-F", "file=@"+file, "--match", "name=(one, two)")
	if err != nil || calls.Load() != 2 || !strings.Contains(out, "Completed: requests=2, errors=0") {
		t.Errorf("calls=%d, error=%v, output:\n%s", calls.Load(), err, out)
	}
}

func TestSendRejectsInvalidMultipartInput(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	for _, flags := range [][]string{
		{"-F", "invalid"},
		{"-F", "bad\nname=value"},
		{"-F", "file=@"},
		{"-F", "file=@missing-file.bin"},
		{"-F", "name=value", "-d", "body"},
		{"-F", "name=value", "--data-file", "body.json"},
		{"-F", "name=value", "-H", "Content-Type: multipart/form-data"},
	} {
		args := append([]string{"send", server.URL}, flags...)
		if _, err := executeSend(nil, args...); err == nil {
			t.Errorf("expected error for %q", flags)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("sent %d requests for invalid form input", calls.Load())
	}
}

func TestSendRejectsInvalidMatchedFormName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("invalid form name sent a request")
	}))
	defer server.Close()
	_, err := executeSend(nil, "send", server.URL, "-F", "{name}=value", "--match", "name=(bad\nname)")
	if err == nil {
		t.Fatal("expected invalid rendered form name")
	}
}
