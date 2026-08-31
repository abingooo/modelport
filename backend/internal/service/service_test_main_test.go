package service

import (
	"os"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

var ginTestModeOnce sync.Once

func setGinModeForTest(mode string) {
	ginTestModeOnce.Do(func() {
		gin.SetMode(mode)
	})
}

func TestMain(m *testing.M) {
	setGinModeForTest(gin.TestMode)
	os.Exit(m.Run())
}
