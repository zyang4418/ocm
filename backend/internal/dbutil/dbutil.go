// Package dbutil holds shared MySQL/database helpers used across the classroom,
// schedule, course, user and importer packages: 1062 duplicate-entry detection
// and index-name extraction. Centralizing them here means a fix to 1062
// handling propagates to all callers instead of drifting across the five
// identical copies that used to live in classroom/store.go, schedule/store.go,
// course/store.go, user/org_store.go and importer/sessions.go.
package dbutil

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// IsDuplicateEntry reports whether err is a MySQL 1062 (duplicate entry)
// unique-constraint violation. Insert/update paths use it to map a 1062 to a
// domain sentinel (ErrNameTaken / ErrCodeTaken / ErrOfferingTaken …) instead of
// surfacing the raw driver error.
func IsDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// DuplicateKeyName inspects a MySQL 1062 (duplicate entry) error and returns the
// index name that caused the collision, e.g. "name" or "uq_catalog_code". It
// returns "" for non-duplicate errors or when the message cannot be parsed.
// Callers use it to map a 1062 to the right sentinel (ErrNameTaken vs
// ErrCodeTaken) instead of guessing from the message.
//
// The message format `Duplicate entry '<val>' for key '<key>'` has been stable
// across MySQL 5.7 / 8.0 / 8.4 (and MariaDB 10.x). mysql.MySQLError carries
// Number/SQLState/Message but no structured constraint name, so parsing Message
// is the only way to recover the index name. If a future server version changes
// the wording, this returns "" and CreateCatalog/UpdateCatalog fall back to
// ErrNameTaken for any 1062 — a wrong-sentinel degradation, not a crash: the
// duplicate is still rejected, just with a possibly inaccurate "name taken"
// message.
func DuplicateKeyName(err error) string {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
		return ""
	}
	// Message format: `Duplicate entry '<val>' for key '<key>'`.
	s := mysqlErr.Message
	i := strings.LastIndex(s, "for key '")
	if i < 0 {
		return ""
	}
	s = s[i+len("for key '"):]
	if j := strings.Index(s, "'"); j >= 0 {
		return s[:j]
	}
	return ""
}
