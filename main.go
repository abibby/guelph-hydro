package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/abibby/guelph-hydro/hydro"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	token := os.Getenv("INFLUXDB_TOKEN")
	url := os.Getenv("INFLUXDB_URL")
	org := os.Getenv("INFLUXDB_ORG")
	bucket := os.Getenv("INFLUXDB_BUCKET")
	client := influxdb2.NewClient(url, token)
	writeAPI := client.WriteAPIBlocking(org, bucket)
	queryAPI := client.QueryAPI(org)

	ctx := context.Background()

	r, err := queryAPI.Query(ctx, `from(bucket: "sensors")
		|> range(start: 0, stop: now())
		|> filter(fn: (r) => r._measurement == "sensor.guelph_hydro_energy" and r._field=="cost")
		|> last()
	`)
	if err != nil {
		log.Fatal(err)
	}
	r.Next()
	var lastRecord time.Time = time.Date(2021, 6, 1, 0, 0, 0, 0, time.Local)
	if r.Record() != nil {
		lastRecord = r.Record().Time()
	}
	if lastRecord.After(startOfDay(time.Now()).Add(time.Hour * -24)) {
		log.Print("no new data")
		return
	}

	log.Printf("Download since %s", lastRecord.Format(time.DateOnly))

	c, err := hydro.New(os.Getenv("ACCOUNT_NUMBER"), os.Getenv("PASSWORD"))
	if err != nil {
		log.Fatal(err)
	}

	start := lastRecord
	finished := false
	for !finished {
		end := start.Add(time.Hour * 24 * 30)
		if end.After(time.Now()) {
			end = time.Now()
			finished = true
		}
		log.Printf("Download from %v to %v", start, end)
		usages, err := c.UsageData(start, end)
		if err != nil {
			log.Fatal(err)
		}

		for _, u := range usages {
			tags := map[string]string{}
			fields := map[string]interface{}{
				"usage": u.Usage,
				"cost":  u.Cost,
				"peak":  u.Peak,
			}
			point := write.NewPoint("sensor.guelph_hydro_energy", tags, fields, u.Time)
			if err := writeAPI.WritePoint(ctx, point); err != nil {
				log.Fatal(err)
			}
		}

		start = end
	}

}

func startOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
