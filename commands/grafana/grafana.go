package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	"github.com/Scrin/siikabot/config"
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

func validUser(ctx context.Context, user string) bool {
	return user == config.Admin || db.IsGrafanaAuthorized(ctx, user)
}

func formatConfigs(configs map[string]db.GrafanaConfig) string {
	respLines := []string{"Current Grafana configs: "}
	for name := range configs {
		respLines = append(respLines, name)
	}
	return strings.Join(respLines, "\n")
}

func formatConfig(config db.GrafanaConfig) string {
	respLines := []string{"Template string: " + config.TemplateString, "Data sources:"}
	for k, v := range config.DataSources {
		respLines = append(respLines, k+" = "+v)
	}
	return strings.Join(respLines, "\n")
}

func formatTemplate(config db.GrafanaConfig) string {
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
	if len(params) == 1 {
		return
	}
	switch params[1] {
	case "help":
		matrix.SendFormattedMessage(roomID, "Usage: <br>"+
			"<b>!grafana config</b> prints the config<br>"+
			"<b>!grafana add &lt;template-name></b> adds a new template config<br>"+
			"<b>!grafana remove &lt;template-name></b> removes a template config<br>"+
			"<b>!grafana rename &lt;template-name></b> renames a template config<br>"+
			"<b>!grafana set template &lt;template-name> &lt;templatestring></b> sets the template string for a template config<br>"+
			"<b>!grafana set datasource &lt;template-name> &lt;datasource-name> &lt;datasource-url></b> sets a datasource for a template config. <b>-</b> as url will remove the datasource")
	case "config":
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		configs, err := db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		if len(params) == 3 {
			config, ok := configs[params[2]]
			if !ok {
				matrix.SendMessage(roomID, "Template "+params[2]+" not found.")
				return
			}
			matrix.SendMessage(roomID, formatConfig(config))
		} else {
			matrix.SendMessage(roomID, formatConfigs(configs))
		}
	case "add":
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 3 {
			matrix.SendMessage(roomID, "Usage: !grafana add <template-name>")
			return
		}
		if err := db.AddGrafanaTemplate(ctx, params[2], ""); err != nil {
			matrix.SendMessage(roomID, "Failed to add template: "+err.Error())
			return
		}
		configs, err := db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatConfigs(configs))
	case "remove":
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 3 {
			matrix.SendMessage(roomID, "Usage: !grafana remove <template-name>")
			return
		}
		if err := db.RemoveGrafanaTemplate(ctx, params[2]); err != nil {
			matrix.SendMessage(roomID, "Failed to remove template: "+err.Error())
			return
		}
		configs, err := db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatConfigs(configs))
	case "rename":
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 4 {
			matrix.SendMessage(roomID, "Usage: !grafana rename <template-name> <new-name>")
			return
		}
		configs, err := db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		config, ok := configs[params[2]]
		if !ok {
			matrix.SendMessage(roomID, "Config "+params[2]+" not found")
			return
		}

		// Add the new template
		if err := db.AddGrafanaTemplate(ctx, params[3], config.TemplateString); err != nil {
			matrix.SendMessage(roomID, "Failed to add new template: "+err.Error())
			return
		}

		// Move all datasources to the new template
		for sourceName, sourceURL := range config.DataSources {
			if err := db.SetGrafanaDatasource(ctx, params[3], sourceName, sourceURL); err != nil {
				matrix.SendMessage(roomID, "Failed to move datasource: "+err.Error())
				return
			}
		}

		// Remove the old template (this will cascade delete its datasources)
		if err := db.RemoveGrafanaTemplate(ctx, params[2]); err != nil {
			matrix.SendMessage(roomID, "Failed to remove old template: "+err.Error())
			return
		}

		configs, err = db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		matrix.SendMessage(roomID, formatConfigs(configs))
	case "set":
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) < 4 {
			matrix.SendMessage(roomID, "Usage: !grafana set [template/datasource] <...>")
			return
		}
		configs, err := db.GetGrafanaConfigs(ctx)
		if err != nil {
			matrix.SendMessage(roomID, "Error getting configs: "+err.Error())
			return
		}
		config, ok := configs[params[3]]
		if !ok {
			matrix.SendMessage(roomID, "Template "+params[3]+" not found. Add it first with !grafana add "+params[3])
			return
		}
		switch params[2] {
		case "template":
			if len(params) < 5 {
				matrix.SendMessage(roomID, "Usage: !grafana set template <template-name> <templatestring>")
				return
			}
			if err := db.AddGrafanaTemplate(ctx, params[3], strings.Join(params[4:], " ")); err != nil {
				matrix.SendMessage(roomID, "Failed to update template: "+err.Error())
				return
			}
			config.TemplateString = strings.Join(params[4:], " ")
			matrix.SendFormattedMessage(roomID, formatTemplate(config))
		case "datasource":
			if len(params) < 6 {
				matrix.SendMessage(roomID, "Usage: !grafana set datasource <template-name> <datasource-name> <datasource-url>")
				return
			}
			if err := db.SetGrafanaDatasource(ctx, params[3], params[4], params[5]); err != nil {
				matrix.SendMessage(roomID, "Failed to set datasource: "+err.Error())
				return
			}
			config.DataSources[params[4]] = params[5]
			matrix.SendFormattedMessage(roomID, formatTemplate(config))
		default:
			matrix.SendMessage(roomID, "Usage: !grafana set [template/datasource]")
		}
	default:
		if !validUser(ctx, sender) {
			matrix.SendMessage(roomID, "Only authorized users can use this command")
			return
		}
		if len(params) == 2 {
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
			matrix.SendFormattedMessage(roomID, formatTemplate(config))
		} else {
			matrix.SendMessage(roomID, "Usage: !grafana <template-name>")
		}
	}
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
