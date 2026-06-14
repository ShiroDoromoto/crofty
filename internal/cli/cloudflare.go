package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// cfAPIBase is the Cloudflare API root. It is a package var so tests can point
// it at an httptest server instead of the network.
var cfAPIBase = "https://api.cloudflare.com/client/v4"

func cfHTTP() *http.Client { return &http.Client{Timeout: 20 * time.Second} }

// cfResponse is the common envelope Cloudflare wraps every v4 reply in.
type cfResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (r cfResponse) err(status int) error {
	if len(r.Errors) > 0 {
		return fmt.Errorf("%s", r.Errors[0].Message)
	}
	return fmt.Errorf("Cloudflare API error (HTTP %d)", status)
}

// cfListAccounts returns the accounts a token can see. A token scoped to a
// single account (the common "Pages: Edit" case) may legitimately return none if
// it lacks account-read; callers then fall back to asking for the account id.
func cfListAccounts(token string) ([]cfAccount, error) {
	body, status, err := cfGet(token, "/accounts")
	if err != nil {
		return nil, err
	}
	var out struct {
		cfResponse
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden || !out.Success {
		return nil, out.err(status)
	}
	var accts []cfAccount
	for _, a := range out.Result {
		accts = append(accts, cfAccount{id: a.ID, name: a.Name})
	}
	return accts, nil
}

// cfVerifyPagesAccess confirms a token can manage Pages on a specific account —
// the exact capability `crofty deploy` needs. Pages: Edit always allows this.
func cfVerifyPagesAccess(token, accountID string) error {
	_, status, err := cfGet(token, "/accounts/"+accountID+"/pages/projects")
	if err != nil {
		return err
	}
	if status >= 200 && status < 300 {
		return nil
	}
	return fmt.Errorf("the token can't manage Pages on account %s (it needs 'Cloudflare Pages: Edit' on that account)", accountID)
}

func cfGet(token, path string) (body []byte, status int, err error) {
	req, err := http.NewRequest(http.MethodGet, cfAPIBase+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := cfHTTP().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := resp.Body.Read(buf)
		b = append(b, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	return b, resp.StatusCode, nil
}
