package list

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	httptransport "github.com/go-kit/kit/transport/http"
)

func NewClient(instance string, logger log.Logger, token string, opts ...httptransport.ClientOption) (Service, error) {
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	var listDevicesEndpoint endpoint.Endpoint
	{
		listDevicesEndpoint = httptransport.NewClient(
			"GET",
			copyURL(u, "/v1/devices"),
			encodeRequestWithToken(token, EncodeHTTPGenericRequest),
			DecodeDevicesResponse,
			opts...,
		).Endpoint()
	}
	var getDEPTokensEndpoint endpoint.Endpoint
	{
		getDEPTokensEndpoint = httptransport.NewClient(
			"GET",
			copyURL(u, "/v1/dep-tokens"),
			encodeRequestWithToken(token, EncodeHTTPGenericRequest),
			DecodeGetDEPTokensResponse,
			opts...,
		).Endpoint()
	}
	var getBlueprintsEndpoint endpoint.Endpoint
	{
		getBlueprintsEndpoint = httptransport.NewClient(
			"GET",
			copyURL(u, "/v1/blueprints"),
			encodeRequestWithToken(token, EncodeHTTPGenericRequest),
			DecodeGetBlueprintsResponse,
		).Endpoint()
	}
	var getProfilesEndpoint endpoint.Endpoint
	{
		getProfilesEndpoint = httptransport.NewClient(
			"GET",
			copyURL(u, "/v1/profiles"),
			encodeRequestWithToken(token, EncodeHTTPGenericRequest),
			DecodeGetProfilesResponse,
		).Endpoint()
	}

	return Endpoints{
		ListDevicesEndpoint:   listDevicesEndpoint,
		GetDEPTokensEndpoint:  getDEPTokensEndpoint,
		GetBlueprintsEndpoint: getBlueprintsEndpoint,
		GetProfilesEndpoint:   getProfilesEndpoint,
	}, nil
}

func encodeRequestWithToken(token string, next httptransport.EncodeRequestFunc) httptransport.EncodeRequestFunc {
	return func(ctx context.Context, r *http.Request, request interface{}) error {
		r.SetBasicAuth("micromdm", token)
		return next(ctx, r, request)
	}
}
func copyURL(base *url.URL, path string) *url.URL {
	next := *base
	next.Path = path
	return &next
}
