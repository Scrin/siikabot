package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
)

type grafanaResponse struct {
	Results []struct {
		Series []struct {
			Values [][]any `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

// FormatTemplate renders a Grafana template by querying all datasources and executing the Go template.
// This is used by both the !grafana command and the web UI preview API.
func FormatTemplate(config db.GrafanaConfig) string {
	tmpl, err := template.New("").Parse(config.TemplateString)
	if err != nil {
		return err.Error()
	}
	values := make(map[string]string)
	for k, v := range config.DataSources {
		values[k] = queryGrafana(v)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, values); err != nil {
		return err.Error()
	}
	return buf.String()
}

// Handle handles the grafana command
func Handle(ctx context.Context, roomID, sender, msg string) {
	params := strings.Split(msg, " ")
	if len(params) != 2 {
		matrix.SendMessage(roomID, "Usage: !grafana <template-name>")
		return
	}

	configs, err := db.GetGrafanaConfigs(ctx)
	if err != nil {
		matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
		return
	}

	config, ok := configs[params[1]]
	if !ok {
		matrix.SendMessage(roomID, "Template "+params[1]+" not found.")
		return
	}

	matrix.SendFormattedMessage(roomID, FormatTemplate(config))
}

func queryGrafana(queryURL string) string {
	resp, err := http.Get(queryURL)
	if err != nil {
		return err.Error()
	}
	var grafanaResp grafanaResponse
	if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
		return err.Error()
	} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
		return "N/A"
	}

	if floatValue, ok := grafanaResp.Results[0].Series[0].Values[0][1].(float64); ok {
		return strconv.FormatFloat(floatValue, 'f', 2, 64)
	} else {
		return grafanaResp.Results[0].Series[0].Values[0][1].(string)
	}
}
