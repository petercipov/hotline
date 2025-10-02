package http_test

import (
	"hotline/http"
	"maps"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HTTP RoutePattern Matching", func() {

	Context("Route", func() {
		It("creates Id from all parts", func() {
			r := http.Route{
				Method:      "GET",
				PathPattern: "/users/{user-id}",
				Host:        "example.com",
				Port:        443,
			}

			key := r.GenerateKey("some salt")
			Expect(key.String()).To(Equal(
				"RKQ4-S4aI1Efk",
			))
		})

		It("creates Id from empty", func() {
			r := http.Route{}
			key := r.GenerateKey("some salt")
			Expect(key.String()).To(Equal(
				"RKBzH5QWiFkG0",
			))
		})
	})

	Context("Just Root Path", func() {
		pattern := http.NewRoutePattern(http.Route{PathPattern: "/"})

		DescribeTable("Matches All except root",
			func(path string) {
				match := pattern.Matches(http.RequestLocator{
					Path: path,
				})
				Expect(match).To(BeTrue())
			},
			Entry("empty", ""),
			Entry("root", "/"),
			Entry("anything", "/users"),
		)
	})

	Context("Simple path", func() {
		pattern := http.NewRoutePattern(http.Route{Method: "Get", PathPattern: "/users"})

		DescribeTable("Mismatches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeFalse())
			},
			Entry("mismatch in http method", "POST", "/users"),
			Entry("mismatch in path", "GET", "/posts"),
			Entry("mismatch in root", "GET", "/"),
		)

		DescribeTable("Matches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeTrue())
			},
			Entry("match", "GET", "/users"),
			Entry("match in sub path", "GET", "/users/1234567890"),
			Entry("match in sub sub path", "GET", "/users/1234567890/logins"),
			Entry("match in anycase path", "GET", "/UsErs"),
			Entry("match in anycase host", "GET", "/users"),
		)
	})

	Context("Named Wildcard", func() {
		pattern := http.NewRoutePattern(http.Route{Method: "GET", PathPattern: "/users/{user-id}"})

		DescribeTable("Mismatches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeFalse())
			},
			Entry("mismatch in short path", "GET", "/users"),
			Entry("mismatch in root", "GET", "/"),
		)

		DescribeTable("Matches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeTrue())
			},
			Entry("match in path", "GET", "/users/1234567890"),
			Entry("match in longer path", "GET", "/users/1234567890/logins"),
		)
	})

	Context("Multi Named Wildcard", func() {
		pattern := http.NewRoutePattern(http.Route{Method: "GET", PathPattern: "/users/{user-id}/logins/{login-id}"})

		DescribeTable("Mismatches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeFalse())
			},
			Entry("mismatch in 1. level", "GET", "/users"),
			Entry("mismatch in 2. level", "GET", "/users/12345"),
			Entry("mismatch in 3. level", "GET", "/users/12345/logins"),
			Entry("mismatch in 3. level slash", "GET", "/users/12345/logins/"),
		)

		DescribeTable("Matches",
			func(method, path string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
				})
				Expect(match).To(BeTrue())
			},
			Entry("match multi wildcard", "GET", "/users/1234567890/logins/L1234567890"),
			Entry("match multi wildcard slash", "GET", "/users/1234567890/logins/L1234567890/"),
			Entry("match multi wildcard sub", "GET", "/users/1234567890/logins/L1234567890/sub"),
		)
	})

	Context("Host", func() {
		pattern := http.NewRoutePattern(http.Route{Method: "GET", PathPattern: "/users", Host: "eXAmple.com"})

		DescribeTable("Mismatches",
			func(method, path, host string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
					Host:   host,
				})
				Expect(match).NotTo(BeTrue())
			},
			Entry("mismatch", "GET", "/users", "other.example.com"),
		)

		DescribeTable("Matches",
			func(method, path, host string) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
					Host:   host,
				})
				Expect(match).To(BeTrue())
			},
			Entry("match", "GET", "/users", "example.com"),
		)
	})

	Context("port", func() {
		pattern := http.NewRoutePattern(http.Route{Method: "GET", PathPattern: "/users", Host: "eXAmple.com", Port: 443})

		DescribeTable("Mismatches",
			func(method, path, host string, port int) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
					Host:   host,
					Port:   port,
				})
				Expect(match).NotTo(BeTrue())
			},
			Entry("mismatch", "GET", "/users", "example.com", 80),
		)

		DescribeTable("Matches",
			func(method, path, host string, port int) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
					Host:   host,
					Port:   port,
				})
				Expect(match).To(BeTrue())
			},
			Entry("match", "GET", "/users", "example.com", 443),
		)
	})

	Context("match missing method", func() {
		pattern := http.NewRoutePattern(http.Route{PathPattern: "/users", Host: "example.com", Port: 443})

		DescribeTable("Matches",
			func(method, path, host string, port int) {
				match := pattern.Matches(http.RequestLocator{
					Method: method,
					Path:   path,
					Host:   host,
					Port:   port,
				})
				Expect(match).To(BeTrue())
			},
			Entry("any match", "GET", "/users", "example.com", 443),
		)
	})
})

var _ = Describe("Mux", func() {
	Context("Request", func() {
		It("will not match if mux is empty", func() {
			mux := http.Mux[string]{}
			result := mux.LocaleHandler(http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "example.com",
				Port:   443,
			})

			Expect(result).To(BeNil())
		})

		It("will match simple path", func() {
			mux := http.Mux[string]{}
			var handler = "handler"
			mux.Upsert(http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}, &handler)

			result := mux.LocaleHandler(http.RequestLocator{
				Method: "GET",
				Path:   "/users",
				Host:   "example.com",
				Port:   443,
			})

			Expect(result).To(Equal(&handler))
		})

		It("will match longer pattern first", func() {
			mux := http.Mux[string]{}
			var handler1 = "handler1"
			var handler2 = "handler2"
			mux.Upsert(http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}, &handler1)
			mux.Upsert(http.Route{Method: "GET", PathPattern: "/users/{user-id}", Host: "example.com", Port: 443}, &handler2)

			result := mux.LocaleHandler(http.RequestLocator{
				Method: "GET",
				Path:   "/users/1234",
				Host:   "example.com",
				Port:   443,
			})

			Expect(result).To(Equal(&handler2))
		})
	})

	Context("Handlers", func() {
		It("will return handlers", func() {
			mux := http.Mux[string]{}
			handler1 := "handler1"
			route1 := http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}
			handler2 := "handler2"
			route2 := http.Route{Method: "GET", PathPattern: "/users/{user-id}", Host: "example.com", Port: 443}
			mux.Upsert(route1, &handler1)
			mux.Upsert(route2, &handler2)

			collected := maps.Collect(mux.Handlers())

			Expect(collected).To(HaveKeyWithValue(route1, handler1))
			Expect(collected).To(HaveKeyWithValue(route2, handler2))
		})

		It("will stop collecting after break", func() {
			mux := http.Mux[string]{}
			handler1 := "handler1"
			route1 := http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}
			handler2 := "handler2"
			route2 := http.Route{Method: "GET", PathPattern: "/users/{user-id}", Host: "example.com", Port: 443}
			mux.Upsert(route1, &handler1)
			mux.Upsert(route2, &handler2)

			for route, handler := range mux.Handlers() {
				Expect(route2).To(Equal(route))
				Expect(handler2).To(Equal(handler))
				break
			}
		})
	})

	Context("Upsert", func() {
		It("will update handler for same route", func() {
			mux := http.Mux[string]{}
			handler1 := "handler1"
			route1 := http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}
			handler2 := "handler2"
			mux.Upsert(route1, &handler1)
			mux.Upsert(route1, &handler2)

			count := 0
			for range mux.Handlers() {
				count++
			}
			Expect(count).To(Equal(1))

			for route, handler := range mux.Handlers() {
				Expect(route).To(Equal(route1))
				Expect(handler).To(Equal(handler2))
			}
		})
	})

	Context("Delete", func() {
		It("will delete handler for same route", func() {
			mux := http.Mux[string]{}
			handler1 := "handler1"
			route1 := http.Route{Method: "GET", PathPattern: "/users", Host: "example.com", Port: 443}
			mux.Upsert(route1, &handler1)
			mux.Delete(route1)

			count := 0
			for range mux.Handlers() {
				count++
			}
			Expect(count).To(Equal(0))
		})
	})
})
