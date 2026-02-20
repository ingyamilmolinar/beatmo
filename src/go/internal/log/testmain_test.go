package log

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("BEATMO_TEST_LOG")
	_ = os.Unsetenv("BEATMO_TEST_LOG_LEVEL")
	os.Exit(m.Run())
}
