package config

import "fmt"

// defaultFormat is the format helper used by the redacted-password
// test. It is split into its own file so the main test file does not
// need to import fmt directly (keeps the diff of the main test file
// easy to read).
func defaultFormat(v any) string {
	return fmt.Sprintf("%+v", v)
}
