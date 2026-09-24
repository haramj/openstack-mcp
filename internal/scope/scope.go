// Package scope holds immutable, operator-configured identity boundaries.
package scope

import (
	"context"
	"os"
)

type Config struct {
	Principal          string
	Role               string
	MemoryFile         string
	AuditFile          string
	Environment        []string
	RequireElicitation bool
	ProtectedPatterns  []string
	AllowSampling      bool
}
type key struct{}

func With(ctx context.Context, c Config) context.Context { return context.WithValue(ctx, key{}, c) }
func From(ctx context.Context) (Config, bool)            { c, ok := ctx.Value(key{}).(Config); return c, ok }
func Env(ctx context.Context) []string {
	if c, ok := From(ctx); ok {
		return append([]string(nil), c.Environment...)
	}
	return os.Environ()
}
