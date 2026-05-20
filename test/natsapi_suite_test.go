package natsapi_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNatsAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NatsAPI Suite")
}
