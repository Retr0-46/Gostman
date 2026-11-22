package cli

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"gostman/internal/collection"
)

func readFormParts(specs []string) ([]collection.FormPart, error) {
	parts := make([]collection.FormPart, 0, len(specs))
	for _, spec := range specs {
		name, value, ok := strings.Cut(spec, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, "\r\n") {
			return nil, fmt.Errorf("invalid form field %q: expected 'name=value' or 'name=@path'", spec)
		}
		part := collection.FormPart{Name: name}
		switch {
		case strings.HasPrefix(value, "@@"):
			part.Value = value[1:]
		case strings.HasPrefix(value, "@"):
			path := value[1:]
			if path == "" {
				return nil, fmt.Errorf("form file %q has no path", name)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read form file %q: %w", path, err)
			}
			part.IsFile = true
			part.Filename = filepath.Base(path)
			part.ContentType = mime.TypeByExtension(filepath.Ext(path))
			if part.ContentType == "" {
				part.ContentType = "application/octet-stream"
			}
			part.Data = data
		default:
			part.Value = value
		}
		parts = append(parts, part)
	}
	return parts, nil
}
