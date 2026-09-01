// Package brand resolves the deployment's display brand (product/school name)
// in one place so downstream forks can rebrand without forking code files.
//
// The value comes from the BRAND_NAME environment variable; when unset it
// defaults to "OCM" so upstream behavior is unchanged. Read once at startup
// (Name) because the process environment is immutable in practice.
package brand

import (
	"os"
	"strings"
)

// DefaultName is the brand used when BRAND_NAME is not set.
const DefaultName = "OCM"

// Name returns the deployment brand, defaulting to DefaultName when BRAND_NAME
// is unset or blank.
func Name() string {
	if v := strings.TrimSpace(os.Getenv("BRAND_NAME")); v != "" {
		return v
	}
	return DefaultName
}
