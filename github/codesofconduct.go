package github

import (
	"context"
	"fmt"
)

type CodesOfConductService service

// CodeOfConduct represents a code of conduct.
type CodeOfConduct struct {
	Name *string `json:"name,omitempty"`
	Key  *string `json:"key,omitempty"`
	URL  *string `json:"url,omitempty"`
	Body *string `json:"body,omitempty"`
}

func (c *CodeOfConduct) String() string {
	return Stringify(c)
}

// ListCodesOfConduct returns all codes of conduct.
//
// GitHub API docs: https://docs.github.com/rest/codes-of-conduct/codes-of-conduct#get-all-codes-of-conduct
func (s *CodesOfConductService) ListCodesOfConduct(ctx context.Context) ([]*CodeOfConduct, *Response, error) {
	req, err := s.client.NewRequest("GET", "codes_of_conduct", nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept header when this API fully launches.
	req.Header.Set("Accept", mediaTypeCodesOfConductPreview)

	var cs []*CodeOfConduct
	resp, err := s.client.Do(ctx, req, &cs)
	if err != nil {
		return nil, resp, err
	}

	return cs, resp, nil
}

// ListCodesOfConduct
// Deprecated: Use CodesOfConductService.ListCodesOfConduct instead
func (c *Client) ListCodesOfConduct(ctx context.Context) ([]*CodeOfConduct, *Response, error) {
	return c.CodesOfConduct.ListCodesOfConduct(ctx)
}

// GetCodeOfConduct returns an individual code of conduct.
//
// GitHub API docs: https://docs.github.com/rest/codes-of-conduct/codes-of-conduct#get-a-code-of-conduct
func (s *CodesOfConductService) GetCodeOfConduct(ctx context.Context, key string) (*CodeOfConduct, *Response, error) {
	u := fmt.Sprintf("codes_of_conduct/%s", key)
	req, err := s.client.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}

	// TODO: remove custom Accept header when this API fully launches.
	req.Header.Set("Accept", mediaTypeCodesOfConductPreview)

	coc := new(CodeOfConduct)
	resp, err := s.client.Do(ctx, req, coc)
	if err != nil {
		return nil, resp, err
	}

	return coc, resp, nil
}

// GetCodeOfConduct
// Deprecated: Use CodesOfConductService.GetCodeOfConduct instead
func (c *Client) GetCodeOfConduct(ctx context.Context, key string) (*CodeOfConduct, *Response, error) {
	return c.CodesOfConduct.GetCodeOfConduct(ctx, key)
}
