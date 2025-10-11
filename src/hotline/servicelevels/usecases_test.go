package servicelevels_test

import (
	"context"
	"errors"
	"hotline/clock"
	"hotline/http"
	"hotline/integrations"
	"hotline/servicelevels"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Use Cases", func() {
	DescribeTable("modify failures",
		func(repoErrName string, publisherErrName string, expectedErr error) {
			sut := usecaseSut{}
			sut.forEmptyUseCase()
			sut.withErrors(repoErrName, publisherErrName)

			_, err := sut.whenModifyRoute()
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(Equal(expectedErr))
			}
		},
		Entry("Read levels err", "GetServiceLevels", "", ErrRepoFailure),
		Entry("Modify levels err", "Modify", "", ErrRepoFailure),
		Entry("publisher err", "", "HandleRouteModified", ErrPublisherFailure),
		Entry("ok", "", "", nil),
	)

	DescribeTable("drop route failures",
		func(repoErrName string, publisherErrName string, expectedErr error) {
			sut := usecaseSut{}
			sut.forEmptyUseCase()
			routeKey, err := sut.whenModifyRoute()
			Expect(err).ToNot(HaveOccurred())
			sut.withErrors(repoErrName, publisherErrName)
			if repoErrName == "UnknownRouteKey" {
				routeKey = "unknown route key"
			}
			err = sut.whenDropRoute(routeKey)

			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(Equal(expectedErr))
			}
		},
		Entry("Read levels err", "GetServiceLevels", "", ErrRepoFailure),
		Entry("drop levels err", "Modify", "", ErrRepoFailure),
		Entry("unknown route key err", "UnknownRouteKey", "", servicelevels.ErrRouteNotFound),
		Entry("publisher err", "", "HandleRouteModified", ErrPublisherFailure),
		Entry("ok", "", "", nil),
	)

	DescribeTable("drop service levels failures",
		func(repoErrName string, publisherErrName string, expectedErr error) {
			sut := usecaseSut{}
			sut.forEmptyUseCase()
			_, err := sut.whenModifyRoute()
			Expect(err).ToNot(HaveOccurred())
			sut.withErrors(repoErrName, publisherErrName)
			err = sut.whenDropServiceLevels()

			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(Equal(expectedErr))
			}
		},
		Entry("Read levels err", "GetServiceLevels", "", ErrRepoFailure),
		Entry("drop levels err", "Drop", "", ErrRepoFailure),
		Entry("publisher err", "", "HandleRouteModified", ErrPublisherFailure),
		Entry("ok", "", "", nil),
	)
})

type usecaseSut struct {
	usecase     *servicelevels.UseCase
	manualClock *clock.ManualClock
	repo        *repoWithFailures
	publisher   *publisherWithFailures
}

func (u *usecaseSut) forEmptyUseCase() {
	u.manualClock = clock.NewManualClock(
		clock.ParseTime("2025-02-22T12:02:10Z"),
		500*time.Microsecond,
	)
	u.repo = &repoWithFailures{}
	u.publisher = &publisherWithFailures{}
	u.usecase = servicelevels.NewUseCase(
		u.repo,
		u.manualClock.Now,
		u.publisher,
	)
}

func (u *usecaseSut) withErrors(repoErrName string, publisherErrName string) {
	u.publisher.errName = publisherErrName
	u.repo.errName = repoErrName
}

func (u *usecaseSut) whenModifyRoute() (http.RouteKey, error) {
	return u.usecase.ModifyRoute(
		context.Background(),
		"integration-id",
		servicelevels.RouteModification{
			Route: http.Route{
				Method:      "GET",
				PathPattern: "/path",
				Host:        "host",
			},
		})
}

func (u *usecaseSut) whenDropRoute(routeKey http.RouteKey) error {
	return u.usecase.DeleteRoute(
		context.Background(),
		"integration-id",
		routeKey,
	)
}

func (u *usecaseSut) whenDropServiceLevels() error {
	return u.usecase.DropServiceLevels(
		context.Background(),
		"integration-id",
	)
}

var ErrRepoFailure = errors.New("repo failure")
var ErrPublisherFailure = errors.New("publisher failure")

type repoWithFailures struct {
	repo    servicelevels.InMemoryRepository
	errName string
}

func (i *repoWithFailures) GetServiceLevels(ctx context.Context, id integrations.ID) (servicelevels.ApiServiceLevels, error) {
	if i.errName == "GetServiceLevels" {
		return servicelevels.ApiServiceLevels{}, ErrRepoFailure
	}
	return i.repo.GetServiceLevels(ctx, id)
}

func (i *repoWithFailures) Modify(ctx context.Context, id integrations.ID, slo servicelevels.ApiServiceLevels) error {
	if i.errName == "Modify" {
		return ErrRepoFailure
	}
	return i.repo.Modify(ctx, id, slo)
}

func (i *repoWithFailures) Drop(ctx context.Context, id integrations.ID) error {
	if i.errName == "Drop" {
		return ErrRepoFailure
	}
	return i.repo.Drop(ctx, id)
}

type publisherWithFailures struct {
	publisher servicelevels.InMemoryEventPublisher
	errName   string
}

func (i *publisherWithFailures) HandleRouteModified(event []servicelevels.ModifyForRouteMessage) error {
	if i.errName == "HandleRouteModified" {
		return ErrPublisherFailure
	}
	return i.publisher.HandleRouteModified(event)
}
