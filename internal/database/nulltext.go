package database

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"

	"gorm.io/gorm/schema"
)

func init() {
	schema.RegisterSerializer("nulltext", nullText{})
}

// nullText keeps a Go string in a nullable column, tagged `gorm:"serializer:nulltext"`:
// an empty string is NULL. Like unixms, GORM applies it to model values only.
type nullText struct{}

func (nullText) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	var value sql.NullString
	if err := value.Scan(dbValue); err != nil {
		return err
	}
	return field.Set(ctx, dst, value.String)
}

func (nullText) Value(_ context.Context, _ *schema.Field, _ reflect.Value, fieldValue any) (any, error) {
	text, ok := fieldValue.(string)
	if !ok {
		return nil, fmt.Errorf("nulltext needs a string field, got %T", fieldValue)
	}
	if text == "" {
		return nil, nil
	}
	return text, nil
}

// NullText is the value of a string in a nullable column, for a Where argument or an
// Updates map, which GORM passes as is: NULL for empty.
func NullText(text string) any {
	if text == "" {
		return nil
	}
	return text
}
