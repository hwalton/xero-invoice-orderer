package service

import (
    "os"
    "testing"
)

func TestMain(m *testing.M) {
    // allow skipping docker-backed tests in CI/dev
    if os.Getenv("DOCKER_DISABLED") == "1" {
        os.Exit(0)
    }
    os.Exit(m.Run())
}