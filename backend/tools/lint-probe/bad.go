package main

import "crypto/md5"

// This file is not part of the auditor module. CI runs gosec on it and expects failure.
func main() {
	md5.New()
}
