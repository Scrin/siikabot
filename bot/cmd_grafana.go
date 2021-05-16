package bot

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type grafanaConfig struct {
	Template string            `json:"template"`
	Sources  map[string]string `json:"sources"`
}

type grafanaResponse struct {
	Results []struct {
		Series []struct {
			Values [][]interface{} `json:"values"`
		} `json:"series"`
	} `json:"results"`
}

func getGrafanaConfigs() map[string]grafanaConfig {
	endpointsJson := db.Get("grafana_configs")
	var configs map[string]grafanaConfig
	if endpointsJson != "" {
		json.Unmarshal([]byte(endpointsJson), &configs)
	}
	if configs == nil {
		configs = make(map[string]grafanaConfig)
	}
	return configs
}

func saveGrafanaConfigs(configs map[string]grafanaConfig) {
	res, err := json.Marshal(configs)
	if err != nil {
		log.Print(err)
		return
	}
	db.Set("grafana_configs", string(res))
}

func getGrafanaUsers() []string {
	endpointsJson := db.Get("grafana_users")
	var users []string
	if endpointsJson != "" {
		json.Unmarshal([]byte(endpointsJson), &users)
	}
	return users
}

func saveGrafanaUsers(users []string) {
	res, err := json.Marshal(users)
	if err != nil {
		log.Print(err)
		return
	}
	db.Set("grafana_users", string(res))
}

func validUser(user string) bool {
	for _, u := range getGrafanaUsers() {
		if u == user {
			return true
		}
	}
	return false
}

func grafana(roomID, sender, msg string) {
	params := strings.Split(msg, " ")
	if len(params) == 1 {
		client.SendMessage(roomID, "Usage: !grafana <template-name>")
		return
	}
	switch params[1] {
	case "help":
		client.SendFormattedMessage(roomID, "Usage: <br>"+
			"<b>!grafana config</b> prints the config<br>"+
			"<b>!grafana add &lt;template-name></b> adds a new template config<br>"+
			"<b>!grafana remove &lt;template-name></b> removes a template config<br>"+
			"<b>!grafana rename &lt;template-name></b> renames a template config<br>"+
			"<b>!grafana set template &lt;template-name> &lt;templatestring></b> sets the template string for a template config<br>"+
			"<b>!grafana set datasource &lt;template-name> &lt;datasource-name> &lt;datasource-url></b> sets a datasource for a template config. <b>-</b> as url will remove the datasource")
	case "config":
		if len(params) == 3 {
			configs := getGrafanaConfigs()
			config, ok := configs[params[2]]
			if !ok {
				client.SendMessage(roomID, "Template "+params[2]+" not found.")
				return
			}
			client.SendMessage(roomID, formatGrafanaConfig(config))
		} else {
			client.SendMessage(roomID, formatGrafanaConfigs(getGrafanaConfigs()))
		}
	case "add":
		if !validUser(sender) {
			client.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 3 {
			client.SendMessage(roomID, "Usage: !grafana add <template-name>")
			return
		}
		configs := getGrafanaConfigs()
		configs[params[2]] = grafanaConfig{"", nil}
		saveGrafanaConfigs(configs)
		client.SendMessage(roomID, formatGrafanaConfigs(configs))
	case "remove":
		if !validUser(sender) {
			client.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 3 {
			client.SendMessage(roomID, "Usage: !grafana remove <template-name>")
			return
		}
		configs := getGrafanaConfigs()
		delete(configs, params[2])
		saveGrafanaConfigs(configs)
		client.SendMessage(roomID, formatGrafanaConfigs(configs))
	case "rename":
		if !validUser(sender) {
			client.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 4 {
			client.SendMessage(roomID, "Usage: !grafana rename <template-name> <new-name>")
			return
		}
		configs := getGrafanaConfigs()
		config, ok := configs[params[2]]
		if !ok {
			client.SendMessage(roomID, "Config "+params[2]+" not found")
			return
		}
		delete(configs, params[2])
		configs[params[3]] = config
		saveGrafanaConfigs(configs)
		client.SendMessage(roomID, formatGrafanaConfigs(configs))
	case "set":
		if !validUser(sender) {
			client.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 4 {
			client.SendMessage(roomID, "Usage: !grafana set [template/datasource] <...>")
			return
		}
		configs := getGrafanaConfigs()
		config, ok := configs[params[3]]
		if !ok {
			client.SendMessage(roomID, "Template "+params[3]+" not found. Add it first with !grafana add "+params[3])
			return
		}
		switch params[2] {
		case "template":
			if len(params) < 5 {
				client.SendMessage(roomID, "Usage: !grafana set template <template-name> <templatestring>")
				return
			}
			config.Template = strings.Join(params[4:], " ")
			configs[params[3]] = config
			saveGrafanaConfigs(configs)
			client.SendFormattedMessage(roomID, formatTemplate(config))
		case "datasource":
			if len(params) < 6 {
				client.SendMessage(roomID, "Usage: !grafana set datasource <template-name> <datasource-name> <datasource-url>")
				return
			}
			if config.Sources == nil {
				config.Sources = make(map[string]string)
			}
			if params[5] == "-" {
				delete(config.Sources, params[4])
			} else {
				config.Sources[params[4]] = params[5]
			}
			configs[params[3]] = config
			saveGrafanaConfigs(configs)
			client.SendFormattedMessage(roomID, formatTemplate(config))
		default:
			client.SendMessage(roomID, "Usage: !grafana set [template/datasource]")
		}
	case "authorize":
		if sender != adminUser {
			client.SendMessage(roomID, "Only admins can use this command")
			return
		}
		if len(params) < 3 {
			client.SendMessage(roomID, "Usage: !grafana authorize <user>")
			return
		}
		users := getGrafanaUsers()
		users = append(users, params[2])
		saveGrafanaUsers(users)
		client.SendMessage(roomID, strings.Join(users, " "))
	default:
		switch len(params) {
		case 2:
			configs := getGrafanaConfigs()
			config, ok := configs[params[1]]
			if !ok {
				client.SendMessage(roomID, "Template "+params[1]+" not found.")
				return
			}
			client.SendFormattedMessage(roomID, formatTemplate(config))
		case 3:
			configs := getGrafanaConfigs()
			config, ok := configs[params[1]]
			if !ok {
				client.SendMessage(roomID, "Template "+params[1]+" not found.")
				return
			}
			if params[2] != "-" {
				client.SendMessage(roomID, "Unknown argument: "+params[2])
				return
			}
			go func() {
				start := time.Now().Unix()
				outChan, done := client.SendStreamingFormattedNotice(roomID)
				for {
					outChan <- formatTemplate(config) + "<br><font color=\"gray\">[last updated at " + time.Now().Format("15:04:05") + "]</font>"
					time.Sleep(10 * time.Second)
					if start+600 < time.Now().Unix() {
						break
					}
				}
				outChan <- formatTemplate(config)
				close(done)
			}()
		default:
			client.SendMessage(roomID, "Usage: !grafana <template-name>")
		}
	}
}

func formatGrafanaConfigs(configs map[string]grafanaConfig) string {
	respLines := []string{"Current Grafana configs: "}
	for name := range configs {
		respLines = append(respLines, name)
	}
	return strings.Join(respLines, "\n")
}

func formatGrafanaConfig(config grafanaConfig) string {
	respLines := []string{"Template string: " + config.Template, "Data sources:"}
	for k, v := range config.Sources {
		respLines = append(respLines, k+" = "+v)
	}
	return strings.Join(respLines, "\n")
}

func formatTemplate(config grafanaConfig) string {
	tmpl, err := template.New("").Parse(config.Template)
	if err != nil {
		return err.Error()
	}
	values := make(map[string]string)
	for k, v := range config.Sources {
		values[k] = queryGrafana(v)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, values); err != nil {
		return err.Error()
	}
	return buf.String()
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
