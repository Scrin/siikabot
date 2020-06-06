package bot

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type ruuviEndpoint struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type grafanaResponse struct {
	Results []struct {
		Series []struct {
			Values [][]interface{} `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

func formatRuuviEndpoints(endpoints []ruuviEndpoint) string {
	respLines := []string{"Current ruuvi endpoints: "}
	for _, endpoint := range endpoints {
		respLines = append(respLines, endpoint.Name+": "+endpoint.URL)
	}
	return strings.Join(respLines, "\n")
}

func getRuuviEndpoints() []ruuviEndpoint {
	endpointsJson := db.Get("ruuvi_endpoints")
	var endpoints []ruuviEndpoint
	if endpointsJson != "" {
		json.Unmarshal([]byte(endpointsJson), &endpoints)
	}
	return endpoints
}

func ruuvi(roomID, sender, msg string) {
	params := strings.Split(msg, " ")
	if len(params) == 1 {
		printRuuviData(roomID)
		return
	}
	switch params[1] {
	case "config":
		client.SendMessage(roomID, formatRuuviEndpoints(getRuuviEndpoints()))
	case "add":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 4 {
			client.SendMessage(roomID, "Usage: !ruuvi add <url> <name>")
			return
		}
		endpoints := append(getRuuviEndpoints(), ruuviEndpoint{strings.Join(params[3:], " "), params[2]})
		res, err := json.Marshal(endpoints)
		if err != nil {
			client.SendMessage(roomID, err.Error())
		}
		db.Set("ruuvi_endpoints", string(res))
		client.SendMessage(roomID, formatRuuviEndpoints(endpoints))
	case "remove":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 3 {
			client.SendMessage(roomID, "Usage: !ruuvi remove <name>")
			return
		}
		endpoints := getRuuviEndpoints()
		var newEndpoints []ruuviEndpoint
		name := strings.Join(params[2:], " ")
		for _, e := range endpoints {
			if e.Name != name {
				newEndpoints = append(newEndpoints, e)
			}
		}
		res, err := json.Marshal(newEndpoints)
		if err != nil {
			client.SendMessage(roomID, err.Error())
		}
		db.Set("ruuvi_endpoints", string(res))
		client.SendMessage(roomID, formatRuuviEndpoints(newEndpoints))
	}
}

func printRuuviData(roomID string) {
	endpoints := getRuuviEndpoints()
	var respLines []string
	for _, e := range endpoints {
		resp, err := http.Get(e.URL)
		if err != nil {
			respLines = append(respLines, e.Name+" error: "+err.Error())
		} else {
			var grafanaResp grafanaResponse
			if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
				respLines = append(respLines, e.Name+" error: "+err.Error())
			} else {
				temps := grafanaResp.Results[0].Series[0].Values
				currentTemp := strconv.FormatFloat(temps[len(temps)-1][1].(float64), 'f', 2, 64)
				respLines = append(respLines, e.Name+": "+currentTemp+"ºC")
			}
		}
	}
	client.SendMessage(roomID, strings.Join(respLines, "\n"))
}
