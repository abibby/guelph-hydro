package hydro

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Usage struct {
	Time  time.Time `json:"time"`
	Usage float64   `json:"usage"`
	Peak  string    `json:"peak"`
	Cost  float64   `json:"cost"`
}

type Client struct {
	http       *http.Client
	jar        *cookiejar.Jar
	cookieFile string

	username string
	password string
}

func New(username, password string) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		http:       &http.Client{Jar: jar},
		jar:        jar,
		cookieFile: "./cookies.json",
		username:   username,
		password:   password,
	}
	b, err := os.ReadFile(c.cookieFile)
	if errors.Is(err, os.ErrNotExist) {
		err = c.login()
		if err != nil {
			return nil, err
		}
		return c, nil
	} else if err != nil {
		return nil, fmt.Errorf("error loading cookie file: %w", err)
	}

	cookies := []*http.Cookie{}
	err = json.Unmarshal(b, &cookies)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse("https://apps.guelphhydro.com/")
	if err != nil {
		return nil, fmt.Errorf("failed to parse root url: %w", err)
	}
	jar.SetCookies(u, cookies)

	return c, nil
}

func (c *Client) login() error {
	body := url.Values{}
	body.Add("acn", c.username)
	body.Add("pass", c.password)
	body.Add("Submit", "Sign-On")

	query := url.Values{}
	query.Add("command", "login")
	query.Add("TokenID", "null")
	query.Add("Reset", "null")

	uri := "https://apps.guelphhydro.com/AccountOnlineWeb/AccountOnlineCommand"
	_, err := c.postForm(uri, query, body)
	if err != nil {
		return err
	}

	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("failed to parse login url: %w", err)
	}
	cookies := c.jar.Cookies(u)

	b, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	err = os.WriteFile(c.cookieFile, b, 0x644)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) postForm(url string, query, body url.Values) (*http.Response, error) {
	log.Printf("POST %s", url)
	queryStr := ""
	if query != nil {
		queryStr = "?" + query.Encode()
	}

	resp, err := c.http.PostForm(url+queryStr, body)
	if err != nil {
		return nil, fmt.Errorf("request failed: %s: %w", url, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		defer resp.Body.Close()
		_, _ = io.Copy(os.Stdout, resp.Body)
		return nil, fmt.Errorf("download: invalid status %s", resp.Status)
	}
	return resp, nil
}

func (c *Client) rawUsageData(usageType string, framing string, start, end time.Time) (*http.Response, error) {
	query := url.Values{}
	query.Add(usageType, "true")
	query.Add("UsageType", usageType)

	body := url.Values{}
	body.Add("StartDate", start.Format(time.DateOnly))
	body.Add("EndDate", end.Format(time.DateOnly))
	if framing != "" {
		body.Add("framing", framing)
	}
	body.Add("Submit", "Submit")

	return c.postForm("https://apps.guelphhydro.com/AccountOnlineWeb/ChartServlet", query, body)
}

func (c *Client) UsageData(start, end time.Time) ([]*Usage, error) {

	resp, err := c.rawUsageData("DownloadRawDataVertical", "TOU", start, end)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	usages := []*Usage{}

	// _, err = io.Copy(os.Stdout, resp.Body)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("%s\n", resp.Status)
	// for key, values := range resp.Header {
	// 	fmt.Printf("%s, %#v\n", key, values)
	// }
	// os.Exit(1)

	r := csv.NewReader(resp.Body)

	// f, err := os.Open("./example.csv")
	// if err != nil {
	// 	return nil, err
	// }
	// r := csv.NewReader(f)

	_, err = r.Read()
	if err != nil {
		return nil, err
	}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		u, err := newUsage(record)
		if err != nil {
			return nil, err
		}
		if u.Usage != 0 {
			usages = append(usages, u)
		}
	}

	// spew.Dump(usages)

	return usages, nil
}

func newUsage(record []string) (*Usage, error) {
	t, err := time.ParseInLocation(time.DateOnly, record[0], time.FixedZone("EST", -18000))
	if err != nil {
		return nil, err
	}
	hour, err := strconv.Atoi(record[1])
	if err != nil {
		return nil, err
	}
	t = t.Add(time.Duration(hour) * time.Hour)

	use, err := strconv.ParseFloat(record[2], 32)
	if err != nil {
		return nil, err
	}
	peak := record[3]
	cost, err := strconv.ParseFloat(record[4], 32)
	if err != nil {
		return nil, err
	}
	return &Usage{
		Time:  t,
		Usage: use,
		Peak:  peak,
		Cost:  cost,
	}, nil
}
