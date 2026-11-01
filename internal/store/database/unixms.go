package database

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"

	"gorm.io/gorm/schema"
)

func init() {
	schema.RegisterSerializer("unixms", unixMillis{})
}

// unixMillis keeps a Go int64 of Unix milliseconds in a timestamptz column, tagged
// `gorm:"serializer:unixms"`, so the database stays readable and the code and JSON
// keep their numbers. Zero is NULL.
//
// GORM applies it to model values only. A Where argument or an Updates map goes to
// PostgreSQL as is, so those pass a time.Time: see FromMillis. A field named UpdatedAt
// or CreatedAt also needs autoUpdateTime:false or autoCreateTime:false, because GORM
// would otherwise fill it with seconds.
type unixMillis struct{}

func (unixMillis) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, dbValue any) error {
	var value sql.NullTime
	if err := value.Scan(dbValue); err != nil {
		return err
	}
	if !value.Valid {
		return field.Set(ctx, dst, int64(0))
	}
	return field.Set(ctx, dst, value.Time.UnixMilli())
}

func (unixMillis) Value(_ context.Context, _ *schema.Field, _ reflect.Value, fieldValue any) (any, error) {
	millis, ok := fieldValue.(int64)
	if !ok {
		return nil, fmt.Errorf("unixms needs an int64 field, got %T", fieldValue)
	}
	return FromMillis(millis), nil
}

// FromMillis is the value of Unix milliseconds in a timestamptz column: NULL for zero.
func FromMillis(millis int64) any {
	if millis == 0 {
		return nil
	}
	return time.UnixMilli(millis).UTC()
}
