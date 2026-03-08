package railway

import "github.com/Khan/genqlient/graphql"

// GQL exposes the internal gql() method for external tests only.
// Production code should use the exported wrapper functions instead.
func (c *Client) GQL() graphql.Client {
	return c.gql()
}
