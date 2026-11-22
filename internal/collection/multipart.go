package collection

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

func renderMultipart(parts []FormPart, bindings map[string]string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		name := replacePlaceholders(part.Name, bindings)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			return nil, "", fmt.Errorf("invalid form field name %q", name)
		}
		if !part.IsFile {
			if err := writer.WriteField(name, replacePlaceholders(part.Value, bindings)); err != nil {
				return nil, "", fmt.Errorf("write form field %q: %w", name, err)
			}
			continue
		}
		disposition := mime.FormatMediaType("form-data", map[string]string{"name": name, "filename": part.Filename})
		if disposition == "" {
			return nil, "", fmt.Errorf("invalid form field or filename for %q", name)
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", disposition)
		header.Set("Content-Type", part.ContentType)
		field, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", fmt.Errorf("create file part %q: %w", name, err)
		}
		if _, err := field.Write(part.Data); err != nil {
			return nil, "", fmt.Errorf("write file part %q: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart body: %w", err)
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}
