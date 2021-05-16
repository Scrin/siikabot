package bot

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ruuviEndpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	TagName string `json:"tag_name"`
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
		client.SendFormattedMessage(roomID, formatRuuviData())
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
	case "-":
		go func() {
			start := time.Now().Unix()
			outChan, done := client.SendStreamingFormattedNotice(roomID)
			for {
				outChan <- formatRuuviData() + "<font color=\"gray\">[last updated at " + time.Now().Format("15:04:05") + "]</font>"
				time.Sleep(10 * time.Second)
				if start+600 < time.Now().Unix() {
					break
				}
			}
			outChan <- formatRuuviData()
			close(done)
		}()
	}
}

func ruuviQueryGrafana(baseURL, tagName string, offset time.Duration, fields ...string) (*grafanaResponse, error) {
	tagName = strings.Replace(tagName, `"`, "", -1)
	var queryBuilder strings.Builder
	queryBuilder.WriteString(baseURL)
	queryBuilder.WriteString(`&q=SELECT%20`)
	first := true
	for _, f := range fields {
		if first {
			first = false
		} else {
			queryBuilder.WriteString(",")
		}
		queryBuilder.WriteString(`last("`)
		queryBuilder.WriteString(strings.Replace(f, `"`, "", -1))
		queryBuilder.WriteString(`")`)
	}
	queryBuilder.WriteString(`%20FROM%20"ruuvi_measurements"%20WHERE%20("name"%20%3D%20%27`)
	queryBuilder.WriteString(tagName)
	queryBuilder.WriteString(`%27)%20AND%20time%20<%3D%20now()%20-%20`)
	queryBuilder.WriteString(strconv.FormatInt(int64(offset/time.Second), 10))
	queryBuilder.WriteString(`s%20AND%20time%20>%3D%20now()%20-%20`)
	queryBuilder.WriteString(strconv.FormatInt(int64((offset+time.Hour)/time.Second), 10))
	queryBuilder.WriteString(`s`)
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

func queryRuuviData(roomID, name, tagName, field string) {
	endpoints := getRuuviEndpoints()
	if name == "" && tagName == "" {
		var respLines []string
		for _, e := range endpoints {
			grafanaResp, err := ruuviQueryGrafana(e.BaseURL, e.TagName, 0, field)
			if err != nil {
				respLines = append(respLines, e.Name+" error: "+err.Error())
			} else {
				allValues := grafanaResp.Results[0].Series[0].Values
				latestValues := allValues[len(allValues)-1]
				value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
				respLines = append(respLines, e.Name+" "+field+": <b>"+value+"</b>")
			}
		}
		client.SendFormattedMessage(roomID, strings.Join(respLines, "<br />"))
	} else {
		ok := false
		for _, e := range endpoints {
			if e.Name != name {
				continue
			}
			grafanaResp, err := ruuviQueryGrafana(e.BaseURL, tagName, 0, field)
			if err != nil {
				client.SendMessage(roomID, err.Error())
			} else {
				allValues := grafanaResp.Results[0].Series[0].Values
				latestValues := allValues[len(allValues)-1]
				value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
				client.SendFormattedMessage(roomID, e.Name+" "+tagName+" "+field+": <b>"+value+"</b>")
			}
			ok = true
			break
		}
		if !ok {
			client.SendMessage(roomID, name+" not found")
		}
	}
}

func formatRuuviData() string {
	endpoints := getRuuviEndpoints()
	var respLines []string
	for _, e := range endpoints {
		current, err := ruuviQueryGrafana(e.BaseURL, e.TagName, 0, "temperature", "humidity", "pressure")
		if err != nil {
			respLines = append(respLines, "<p>"+e.Name+" error: "+err.Error()+"</p>")
			continue
		}
		hourAgo, err := ruuviQueryGrafana(e.BaseURL, e.TagName, time.Hour, "temperature")
		if err != nil {
			respLines = append(respLines, "<p>"+e.Name+" error: "+err.Error()+"</p>")
			continue
		}
		yesterday, err := ruuviQueryGrafana(e.BaseURL, e.TagName, 24*time.Hour, "temperature")
		if err != nil {
			respLines = append(respLines, "<p>"+e.Name+" error: "+err.Error()+"</p>")
			continue
		}
		currentValues := current.Results[0].Series[0].Values[0]
		hourAgoValues := hourAgo.Results[0].Series[0].Values[0]
		yesterdayValues := yesterday.Results[0].Series[0].Values[0]
		temp := strconv.FormatFloat(currentValues[1].(float64), 'f', 2, 64)
		humi := strconv.FormatFloat(currentValues[2].(float64), 'f', 2, 64)
		press := strconv.FormatFloat(currentValues[3].(float64)/100, 'f', 2, 64)
		lastHourTemp := strconv.FormatFloat(hourAgoValues[1].(float64), 'f', 2, 64)
		yesterdayTemp := strconv.FormatFloat(yesterdayValues[1].(float64), 'f', 2, 64)
		lastHourDelta := strconv.FormatFloat(currentValues[1].(float64)-hourAgoValues[1].(float64), 'f', 2, 64)
		yesterdayDelta := strconv.FormatFloat(currentValues[1].(float64)-yesterdayValues[1].(float64), 'f', 2, 64)
		respLines = append(respLines, "<span>"+e.Name+": <b>"+temp+"</b> ºC, <b>"+humi+"</b> %, <b>"+press+"</b> hPa</span><ul>"+
			"<li>1h ago: <b>"+lastHourTemp+"</b> ºC (changed <b>"+lastHourDelta+"</b> ºC since 1h ago)</li>"+
			"<li>24h ago: <b>"+yesterdayTemp+"</b> ºC (changed <b>"+yesterdayDelta+"</b> ºC since yesterday)</li></ul>")
	}
	return strings.Join(respLines, "")
}
