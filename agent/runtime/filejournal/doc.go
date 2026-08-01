// Package filejournal persists Agent runtime transactions in checksummed JSONL
// generations with exclusive binding leases, bounded checkpoint tails, and a
// durable command-idempotency index.
package filejournal
