package bot

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type grafanaEndpoint struct {
	Name            string `json:"name"`
	BaseURL         string `json:"base_url"`
	ProxyID         string `json:"proxy_id"`
	DBName          string `json:"db_name"`
	MeasurementName string `json:"measurement_name"`
	SelectorTagKey  string `json:"selector_tag_key"`
}

type grafanaResponse struct {
	Results []struct {
		Series []struct {
			Values [][]interface{} `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

func formatGrafanaEndpoints(endpoints []grafanaEndpoint) string {
	respLines := []string{"Current Grafana endpoints: "}
	for _, endpoint := range endpoints {
		respLines = append(respLines, endpoint.Name+": "+endpoint.BaseURL+
			" proxy_id: "+endpoint.ProxyID+
			" db_name: "+endpoint.DBName+
			" measurement_name: "+endpoint.MeasurementName+
			" selector_tag_key: "+endpoint.SelectorTagKey)
	}
	return strings.Join(respLines, "\n")
}

func getGrafanaEndpoints() []grafanaEndpoint {
	endpointsJson := db.Get("grafana_endpoints")
	var endpoints []grafanaEndpoint
	if endpointsJson != "" {
		json.Unmarshal([]byte(endpointsJson), &endpoints)
	}
	return endpoints
}

func grafana(roomID, sender, msg string) {
	params := strings.Split(msg, " ")
	if len(params) == 1 {
		client.SendMessage(roomID, "Usage: !grafana <name> <measurement_name> <selector> <field>")
		return
	}
	switch params[1] {
	case "config":
		client.SendMessage(roomID, formatGrafanaEndpoints(getGrafanaEndpoints()))
	case "add":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 8 {
			client.SendMessage(roomID, "Usage: !grafana add <name> <base_url> <proxy_id> <db_name> <measurement_name> <selector_tag_key>")
			return
		}
		endpoints := append(getGrafanaEndpoints(), grafanaEndpoint{params[2], params[3], params[4], params[5], params[6], params[7]})
		res, err := json.Marshal(endpoints)
		if err != nil {
			client.SendMessage(roomID, err.Error())
		}
		db.Set("grafana_endpoints", string(res))
		client.SendMessage(roomID, formatGrafanaEndpoints(endpoints))
	case "remove":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 4 {
			client.SendMessage(roomID, "Usage: !grafana remove <name> <db_name>")
			return
		}
		endpoints := getGrafanaEndpoints()
		var newEndpoints []grafanaEndpoint
		for _, e := range endpoints {
			if e.Name != params[2] || e.DBName != params[3] {
				newEndpoints = append(newEndpoints, e)
			}
		}
		res, err := json.Marshal(newEndpoints)
		if err != nil {
			client.SendMessage(roomID, err.Error())
		}
		db.Set("grafana_endpoints", string(res))
		client.SendMessage(roomID, formatGrafanaEndpoints(newEndpoints))
	default:
		if len(params) == 5 {
			queryGrafanaData(roomID, params[1], params[2], params[3], params[4])
		} else {
			client.SendMessage(roomID, "Usage: !grafana <name> <db_name> <selector> <field>")
		}
	}
}

func queryGrafana(endpoint grafanaEndpoint, selector, field string) (*grafanaResponse, error) {
	var queryBuilder strings.Builder
	queryBuilder.WriteString(endpoint.BaseURL)
	queryBuilder.WriteString(`/api/datasources/proxy/`)
	queryBuilder.WriteString(endpoint.ProxyID)
	queryBuilder.WriteString(`/query?db=`)
	queryBuilder.WriteString(endpoint.DBName)
	queryBuilder.WriteString(`&q=SELECT%20last("`)
	queryBuilder.WriteString(strings.Replace(field, `"`, "", -1))
	queryBuilder.WriteString(`")%20FROM%20"`)
	queryBuilder.WriteString(endpoint.MeasurementName)
	queryBuilder.WriteString(`"%20WHERE%20("`)
	queryBuilder.WriteString(endpoint.SelectorTagKey)
	queryBuilder.WriteString(`"%20%3D%20%27`)
	queryBuilder.WriteString(strings.Replace(selector, `"`, "", -1))
	queryBuilder.WriteString(`%27)%20AND%20time%20>%3D%20now()%20-%201h`)
	resp, err := http.Get(queryBuilder.String())
	if err != nil {
		return nil, err
	}
	var grafanaResp grafanaResponse
	if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
		return nil, err
	} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
		return nil, errors.New("No data")
	}
	return &grafanaResp, nil

}

func queryGrafanaData(roomID, name, dbName, selector, field string) {
	endpoints := getGrafanaEndpoints()
	ok := false
	for _, e := range endpoints {
		if e.Name != name || e.DBName != dbName {
			continue
		}
		grafanaResp, err := queryGrafana(e, selector, field)
		if err != nil {
			client.SendMessage(roomID, err.Error())
		} else {
			value := strconv.FormatFloat(grafanaResp.Results[0].Series[0].Values[0][1].(float64), 'f', 2, 64)
			client.SendFormattedMessage(roomID, name+" "+dbName+" "+selector+" "+field+": <b>"+value+"</b>")
		}
		ok = true
		break
	}
	if !ok {
		client.SendMessage(roomID, name+" not found")
	}
}
