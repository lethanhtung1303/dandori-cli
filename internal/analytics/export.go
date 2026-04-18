package analytics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

func ExportCSV(data any, w io.Writer) error {
	val := reflect.ValueOf(data)
	if val.Kind() != reflect.Slice {
		return fmt.Errorf("data must be a slice")
	}

	if val.Len() == 0 {
		return nil
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Get headers from struct fields
	elemType := val.Index(0).Type()
	headers := make([]string, 0, elemType.NumField())
	for i := 0; i < elemType.NumField(); i++ {
		field := elemType.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			tag = field.Name
		} else {
			// Remove omitempty suffix
			for j, c := range tag {
				if c == ',' {
					tag = tag[:j]
					break
				}
			}
		}
		headers = append(headers, tag)
	}

	if err := writer.Write(headers); err != nil {
		return err
	}

	// Write data rows
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		row := make([]string, 0, elem.NumField())
		for j := 0; j < elem.NumField(); j++ {
			field := elem.Field(j)
			row = append(row, fmt.Sprintf("%v", field.Interface()))
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

func ExportJSON(data any, w io.Writer) error {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Slice && val.IsNil() {
		_, err := w.Write([]byte("[]"))
		return err
	}
	if val.Kind() == reflect.Slice && val.Len() == 0 {
		_, err := w.Write([]byte("[]"))
		return err
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
