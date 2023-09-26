package hydro

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Usage struct {
	Time  time.Time `json:"time"`
	Usage float32   `json:"usage"`
	Peak  string    `json:"peak"`
	Cost  float32   `json:"cost"`
}

func Get(start, end time.Time) ([]*Usage, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Jar: jar,
	}

	body := url.Values{}
	body.Add("acn", os.Getenv("ACCOUNT_NUMBER"))
	body.Add("pass", os.Getenv("PASSWORD"))
	resp, err := client.PostForm("https://apps.guelphhydro.com/AccountOnlineWeb/AccountOnlineCommand?command=login&TokenID=null", body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return nil, fmt.Errorf("login: invalid status %s", resp.Status)
	}

	body = url.Values{}
	body.Add("StartDate", start.Format(time.DateOnly))
	body.Add("EndDate", end.Format(time.DateOnly))
	body.Add("Submit", "Submit")
	body.Add("framing", "TOU")

	resp, err = client.PostForm("https://apps.guelphhydro.com/AccountOnlineWeb/ChartServlet?DownloadRawDataVertical=true&UsageType=DownloadRawDataVertical", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		return nil, fmt.Errorf("download: invalid status %s", resp.Status)
	}

	usages := []*Usage{}

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
		cost, err := strconv.ParseFloat(record[4], 32)
		if err != nil {
			return nil, err
		}
		if use > 0 {
			usages = append(usages, &Usage{
				Time:  t,
				Usage: float32(use),
				Peak:  record[3],
				Cost:  float32(cost),
			})
		}
	}

	return usages, nil
}
