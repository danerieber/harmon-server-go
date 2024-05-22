package main

import "os"

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

var cwd, _ = os.Getwd()
var DataDir = getEnv("DATA_DIR", cwd+"/data")
