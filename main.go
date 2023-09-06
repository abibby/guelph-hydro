package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/abibby/guelph-hydro/hydro"
	"github.com/davecgh/go-spew/spew"
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
	lastRecord := r.Record().Time()
	spew.Dump(lastRecord)

	usages, err := hydro.Get(
		lastRecord,
		time.Now(),
	)
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
}

type SensorAttributes struct {
	UnitOfMeasurement string `json:"unit_of_measurement"`
	DeviceClass       string `json:"device_class"`
	StateClass        string `json:"state_class"`
	// LastUpdated       time.Time `json:"last_updated"`
	LastRest time.Time `json:"last_reset"`
	// FriendlyName      string `json:"friendly_name"`
}
type SensorState struct {
	State    float32 `json:"state"`
	UniqueID string  `json:"unique_id"`
	// LastUpdated time.Time         `json:"last_updated"`
	Attributes *SensorAttributes `json:"attributes"`
}

func CreateSensor(name string) error {
	state := &SensorState{
		UniqueID: name,
		Attributes: &SensorAttributes{
			UnitOfMeasurement: "kWh",
			DeviceClass:       "energy",
			StateClass:        "measurement",
			LastRest:          time.Time{},
		},
	}

	r, err := haPost("/api/states/sensor."+name, state)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return nil
}

type EventState struct {
	EntityID    string    `json:"entity_id"`
	State       float32   `json:"state"`
	LastUpdated time.Time `json:"last_updated"`
}
type EventData struct {
	EntityID string      `json:"entity_id"`
	NewState *EventState `json:"new_state"`
}
type Event struct {
	EventData *EventData `json:"event_data"`
}

// event_type: state_changed
//   event_data:
//     entity_id: sensor.3c71bf4822b8_i2saccoef
//     new_state:
//       entity_id: sensor.3c71bf4822b8_i2saccoef
//       state: "1.4"
//       last_updated: "2022-07-25T23:00:25.082925+00:00"

func StateChange(entityID string, u *hydro.Usage) error {
	event := &Event{
		EventData: &EventData{
			EntityID: entityID,
			NewState: &EventState{
				EntityID:    entityID,
				State:       u.Usage,
				LastUpdated: u.Time,
			},
		},
	}

	eventType := "state_changed"

	r, err := haPost("/api/events/"+eventType, event)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return nil
}

func haPost(pathName string, body any) (*http.Response, error) {

	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(
		http.MethodPost,
		"https://"+path.Join("home-assistant.adambibby.ca", pathName),
		bytes.NewBuffer(b),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("LONG_LIVED_ACCESS_TOKEN")))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("request failed %s", resp.Status)
		}
		return nil, fmt.Errorf("request failed %s: %s", resp.Status, b)
	}
	return resp, nil
}
