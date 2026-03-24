package registry_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/registry"
)

var _ = Describe("FetchREADME", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("fetches raw README content from GitHub API", func() {
		readmeContent := "# My Strategy\n\nThis is a test strategy."
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			Expect(req.URL.Path).To(Equal("/repos/alice/momentum/readme"))
			Expect(req.Header.Get("Accept")).To(Equal("application/vnd.github.raw+json"))
			fmt.Fprint(writer, readmeContent)
		}))
		defer server.Close()

		opts := registry.ReadmeOptions{BaseURL: server.URL}
		content, err := registry.FetchREADME(ctx, "alice", "momentum", opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(Equal(readmeContent))
	})

	It("returns error on non-200 response", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
			fmt.Fprint(writer, `{"message": "Not Found"}`)
		}))
		defer server.Close()

		opts := registry.ReadmeOptions{BaseURL: server.URL}
		_, err := registry.FetchREADME(ctx, "alice", "missing", opts)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("404"))
	})

	It("returns error on network failure", func() {
		opts := registry.ReadmeOptions{BaseURL: "http://127.0.0.1:1"}
		_, err := registry.FetchREADME(ctx, "alice", "repo", opts)
		Expect(err).To(HaveOccurred())
	})
})
