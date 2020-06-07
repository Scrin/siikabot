package bot

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type ruuviEndpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	TagName string `json:"tag_name"`
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
		respLines = append(respLines, endpoint.Name+": "+endpoint.BaseURL+" tag name: "+endpoint.TagName)
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
	case "query":
		switch len(params) {
		case 3:
			queryRuuviData(roomID, "", "", params[2])
		case 5:
			queryRuuviData(roomID, params[2], params[3], params[4])
		default:
			client.SendMessage(roomID, "Usage: !ruuvi query [<name> <tag_name>] <field>")
		}
	case "config":
		client.SendMessage(roomID, formatRuuviEndpoints(getRuuviEndpoints()))
	case "add":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 4 {
			client.SendMessage(roomID, "Usage: !ruuvi add <base_url> <tag_name> <name>")
			return
		}
		endpoints := append(getRuuviEndpoints(), ruuviEndpoint{strings.Join(params[4:], " "), params[2], params[3]})
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

func queryRuuviData(roomID, name, tagName, field string) {
	tagName = strings.Replace(tagName, `"`, "", -1)
	field = strings.Replace(field, `"`, "", -1)
	endpoints := getRuuviEndpoints()
	if name == "" && tagName == "" {
		var respLines []string
		for _, e := range endpoints {
			resp, err := http.Get(e.BaseURL + `&q=SELECT%20last("` + field + `")%20FROM%20"ruuvi_measurements"%20WHERE%20("name"%20%3D%20%27` + e.TagName + `%27)%20AND%20time%20>%3D%20now()%20-%201h`)
			if err != nil {
				respLines = append(respLines, e.Name+" error: "+err.Error())
			} else {
				var grafanaResp grafanaResponse
				if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
					respLines = append(respLines, e.Name+" error: "+err.Error())
				} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
					respLines = append(respLines, e.Name+": No data")
				} else {
					allValues := grafanaResp.Results[0].Series[0].Values
					latestValues := allValues[len(allValues)-1]
					value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
					respLines = append(respLines, e.Name+" "+field+": <b>"+value+"</b>")
				}
			}
		}
		client.SendFormattedMessage(roomID, strings.Join(respLines, "<br />"))
	} else {
		ok := false
		for _, e := range endpoints {
			if e.Name != name {
				continue
			}
			resp, err := http.Get(e.BaseURL + `&q=SELECT%20last("` + field + `")%20FROM%20"ruuvi_measurements"%20WHERE%20("name"%20%3D%20%27` + tagName + `%27)%20AND%20time%20>%3D%20now()%20-%201h`)
			if err != nil {
				client.SendFormattedMessage(roomID, err.Error())
			} else {
				var grafanaResp grafanaResponse
				if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
					client.SendFormattedMessage(roomID, err.Error())
				} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
					client.SendFormattedMessage(roomID, "No data")
				} else {
					allValues := grafanaResp.Results[0].Series[0].Values
					latestValues := allValues[len(allValues)-1]
					value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
					client.SendFormattedMessage(roomID, e.Name+" "+tagName+" "+field+": <b>"+value+"</b>")
				}
			}
			ok = true
			break
		}
		if !ok {
			client.SendFormattedMessage(roomID, "No data")
		}
	}
}

func printRuuviData(roomID string) {
	endpoints := getRuuviEndpoints()
	var respLines []string
	for _, e := range endpoints {
		resp, err := http.Get(e.BaseURL + `&q=SELECT%20last("temperature"),%20last("humidity"),%20last("pressure")%20FROM%20"ruuvi_measurements"%20WHERE%20("name"%20%3D%20%27` + e.TagName + `%27)%20AND%20time%20>%3D%20now()%20-%201h`)
		if err != nil {
			respLines = append(respLines, e.Name+" error: "+err.Error())
		} else {
			var grafanaResp grafanaResponse
			if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
				respLines = append(respLines, e.Name+" error: "+err.Error())
			} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
				respLines = append(respLines, e.Name+": No data")
			} else {
				allValues := grafanaResp.Results[0].Series[0].Values
				latestValues := allValues[len(allValues)-1]
				temp := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
				humi := strconv.FormatFloat(latestValues[2].(float64), 'f', 2, 64)
				press := strconv.FormatFloat(latestValues[3].(float64)/100, 'f', 2, 64)
				respLines = append(respLines, e.Name+": <b>"+temp+"</b> ºC, <b>"+humi+"</b> %, <b>"+press+"</b> hPa")
			}
		}
	}
	client.SendFormattedMessage(roomID, strings.Join(respLines, "<br />"))
}
