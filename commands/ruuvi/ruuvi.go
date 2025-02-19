package ruuvi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Scrin/siikabot/db"
	"github.com/Scrin/siikabot/matrix"
)

type endpoint struct {
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

var adminUser string

// Init initializes the ruuvi command with the admin user
func Init(admin string) {
	adminUser = admin
}

func formatEndpoints(endpoints []db.RuuviEndpoint) string {
	respLines := []string{"Current ruuvi endpoints: "}
	for _, endpoint := range endpoints {
		respLines = append(respLines, endpoint.Name+": "+endpoint.BaseURL+" tag name: "+endpoint.TagName)
	}
	return strings.Join(respLines, "\n")
}

// Handle handles the ruuvi command
func Handle(roomID, sender, msg string) {
	ctx := context.Background()
	params := strings.Split(msg, " ")
	if len(params) == 1 {
		matrix.SendFormattedMessage(roomID, formatRuuviData())
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
			matrix.SendMessage(roomID, "Usage: !ruuvi query [<n> <tag_name>] <field>")
		}
	case "config":
		endpoints, err := db.GetRuuviEndpoints(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting endpoints: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatEndpoints(endpoints))
	case "add":
		if sender != adminUser {
			matrix.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 4 {
			matrix.SendMessage(roomID, "Usage: !ruuvi add <base_url> <tag_name> <name>")
			return
		}
		endpoint := db.RuuviEndpoint{
			Name:    strings.Join(params[4:], " "),
			BaseURL: params[2],
			TagName: params[3],
		}
		if err := db.AddRuuviEndpoint(ctx, endpoint); err != nil {
			matrix.SendMessage(roomID, "Error adding endpoint: "+err.Error())
			return
		}
		endpoints, err := db.GetRuuviEndpoints(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting endpoints: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatEndpoints(endpoints))
	case "remove":
		if sender != adminUser {
			matrix.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 3 {
			matrix.SendMessage(roomID, "Usage: !ruuvi remove <n>")
			return
		}
		name := strings.Join(params[2:], " ")
		if err := db.RemoveRuuviEndpoint(ctx, name); err != nil {
			matrix.SendMessage(roomID, "Error removing endpoint: "+err.Error())
			return
		}
		endpoints, err := db.GetRuuviEndpoints(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting endpoints: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatEndpoints(endpoints))
	case "-":
		go func() {
			start := time.Now().Unix()
			outChan, done := matrix.SendStreamingFormattedNotice(roomID)
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

func queryGrafana(baseURL, tagName string, offset time.Duration, fields ...string) (*grafanaResponse, error) {
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
		if err.Error() != "EOF" {
			return nil, err
		}
		resp, err = http.Get(queryBuilder.String())
	}
	if err != nil {
		return nil, err
	}
	var grafanaResp grafanaResponse
	if err = json.NewDecoder(resp.Body).Decode(&grafanaResp); err != nil {
		return nil, err
	} else if len(grafanaResp.Results) < 1 || len(grafanaResp.Results[0].Series) < 1 {
		return nil, errors.New("no data")
	}
	return &grafanaResp, nil
}

func queryRuuviData(roomID, name, tagName, field string) {
	ctx := context.Background()
	endpoints, err := db.GetRuuviEndpoints(ctx)
	if err != nil {
		matrix.SendMessage(roomID, "Error getting endpoints: "+err.Error())
		return
	}

	if name == "" && tagName == "" {
		var respLines []string
		for _, e := range endpoints {
			grafanaResp, err := queryGrafana(e.BaseURL, e.TagName, 0, field)
			if err != nil {
				respLines = append(respLines, e.Name+" error: "+err.Error())
			} else {
				allValues := grafanaResp.Results[0].Series[0].Values
				latestValues := allValues[len(allValues)-1]
				value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
				respLines = append(respLines, e.Name+" "+field+": <b>"+value+"</b>")
			}
		}
		matrix.SendFormattedMessage(roomID, strings.Join(respLines, "<br />"))
	} else {
		ok := false
		for _, e := range endpoints {
			if e.Name != name {
				continue
			}
			grafanaResp, err := queryGrafana(e.BaseURL, tagName, 0, field)
			if err != nil {
				matrix.SendMessage(roomID, err.Error())
			} else {
				allValues := grafanaResp.Results[0].Series[0].Values
				latestValues := allValues[len(allValues)-1]
				value := strconv.FormatFloat(latestValues[1].(float64), 'f', 2, 64)
				matrix.SendFormattedMessage(roomID, e.Name+" "+tagName+" "+field+": <b>"+value+"</b>")
			}
			ok = true
			break
		}
		if !ok {
			matrix.SendMessage(roomID, name+" not found")
		}
	}
}

func formatRuuviData() string {
	ctx := context.Background()
	endpoints, err := db.GetRuuviEndpoints(ctx)
	if err != nil {
		return "Error getting endpoints: " + err.Error()
	}

	var respLines []string
	for _, e := range endpoints {
		current, err := queryGrafana(e.BaseURL, e.TagName, 0, "temperature", "humidity", "pressure")
		if err != nil {
			respLines = append(respLines, "<p>"+e.Name+" error: "+err.Error()+"</p>")
			continue
		}
		hourAgo, err := queryGrafana(e.BaseURL, e.TagName, time.Hour, "temperature")
		if err != nil {
			respLines = append(respLines, "<p>"+e.Name+" error: "+err.Error()+"</p>")
			continue
		}
		yesterday, err := queryGrafana(e.BaseURL, e.TagName, 24*time.Hour, "temperature")
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
