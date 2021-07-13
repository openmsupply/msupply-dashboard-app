package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/simple-datasource-backend/pkg/auth"
)

type Column struct {
	Text string `json:"text"`
}

type TablePanel struct {
	ID           int             `json:"id"`
	From         string          `json:"from"`
	To           string          `json:"to"`
	Title        string          `json:"title"`
	RawSql       string          `json:"rawSql"`
	Rows         [][]interface{} `json:"rows"`
	Columns      []Column        `json:"columns"`
	Variables    TemplateList    `json:"variables"`
	DatasourceID int             `json:"DatasourceID"`
}

func NewTablePanel(id int, title string, rawSql string, from string, to string, datasourceID int) *TablePanel {
	return &TablePanel{ID: id, Title: title, RawSql: rawSql, From: from, To: to, DatasourceID: datasourceID}
}

func (panel *TablePanel) usesVariable(variable TemplateVariable) bool {
	return strings.Contains(panel.RawSql, "${"+variable.Name+"}") || strings.Contains(panel.RawSql, "${"+variable.Name+":sqlstring}")
}

func (panel *TablePanel) injectMacros() {
	usesFrom := strings.Contains(panel.RawSql, "$__timeFrom()")
	usesTo := strings.Contains(panel.RawSql, "$__timeTo()")
	usesTimeFilter := strings.Contains(panel.RawSql, "$__timeFilter(")

	if usesFrom {
		newSql := "to_timestamp(" + panel.To + ")"
		panel.RawSql = strings.Replace(panel.RawSql, "$__timeTo()", newSql, -1)
	}

	if usesTo {
		newSql := "to_timestamp(" + panel.From + ")"
		panel.RawSql = strings.Replace(panel.RawSql, "$__timeFrom()", newSql, -1)
	}

	if usesTimeFilter {
		timeFilter := regexp.MustCompile(`\$__timeFilter\([a-z]+\)`)
		column := regexp.MustCompile(`\([a-zA-Z]+\)`)
		columnName := regexp.MustCompile(`[a-zA-Z]+`)

		timeFilterString := timeFilter.FindString(panel.RawSql)
		columnString := column.FindString(timeFilterString)
		columnNameString := columnName.FindString(columnString)

		newSql := columnNameString + " BETWEEN " + "to_timestamp(" + panel.From + ") AND " + "to_timestamp(" + panel.To + ")"
		panel.RawSql = timeFilter.ReplaceAllString(panel.RawSql, newSql)
	}

	log.DefaultLogger.Info("RawSQL After Injecting Macros (" + panel.Title + "):" + panel.RawSql)
}

func formatSqlString(vars []string) string {
	var formatted string
	for i, str := range vars {
		if i == len(vars)-1 {
			formatted = formatted + "'" + str + "'"
		} else {
			formatted = formatted + "'" + str + "'" + ", "
		}
	}

	return formatted
}

func formatDefaultString(vars []string) string {
	var formatted string
	for i, str := range vars {
		if i == len(vars)-1 {
			formatted = formatted + str
		} else {
			formatted = formatted + str + ", "
		}
	}

	return formatted
}

func formatVariable(variables []string, format string) string {
	switch format {
	case "sqlstring":
		return formatSqlString(variables)
	default:
		return formatDefaultString(variables)
	}
}

func (panel *TablePanel) GetSelectedVariableOptions(variableName, contentVariables string) []string {
	var vars map[string][]string
	err := json.Unmarshal([]byte(contentVariables), &vars)
	if err != nil {
		log.DefaultLogger.Error("GetSelectedVariableOptions: Decoding JSON: "+contentVariables, err.Error())
	}

	variableOptions := vars[variableName]

	return variableOptions
}

func (panel *TablePanel) injectVariable(variable TemplateVariable, contentVariables string) {
	var vars map[string][]string
	err := json.Unmarshal([]byte(contentVariables), &vars)
	if err != nil {
		log.DefaultLogger.Error("injectVariable: Decoding JSON: "+contentVariables, err.Error())
	}

	variableOptions := panel.GetSelectedVariableOptions(variable.Name, contentVariables)

	format := "default"
	if strings.Contains(panel.RawSql, "${"+variable.Name+":sqlstring}") {
		format = "sqlstring"
	}
	formatted := formatVariable(variableOptions, format)

	panel.RawSql = strings.Replace(panel.RawSql, "${"+variable.Name+"}", formatted, -1)
	panel.RawSql = strings.Replace(panel.RawSql, "${"+variable.Name+":sqlstring}", formatted, -1)

	log.DefaultLogger.Info("RawSQL For Panel (" + panel.Title + "):" + panel.RawSql)
}

func (panel *TablePanel) PrepSql(variables TemplateList, contentVariables string) {
	log.DefaultLogger.Info(contentVariables)
	for _, variable := range variables.List {
		if panel.usesVariable(variable) {
			panel.injectVariable(variable, contentVariables)
			panel.injectMacros()
		}
	}
}

func (panel *TablePanel) GetData(authConfig auth.AuthConfig) error {
	log.DefaultLogger.Debug("Panel.GetData");
	body, err := NewQueryRequest(panel.RawSql, panel.From, panel.To, panel.DatasourceID).ToRequestBody()
	if err != nil {
		log.DefaultLogger.Error("GetData: NewQueryRequest: " + err.Error())
		return err
	}

	url := authConfig.AuthURL() + "/api/tsdb/query"
	log.DefaultLogger.Debug(fmt.Sprintf("GetData: url: %s", url));
	response, err := http.Post(url, "application/json", body)
	if err != nil {
		log.DefaultLogger.Error("GetData: http.Post: " + err.Error())
		return err
	}

	qr, err := NewQueryResponse(response)
	if err != nil {
		log.DefaultLogger.Error("GetData: NewQueryResponse: " + err.Error())
		return err
	}

	panel.SetRows(qr.Rows())
	panel.SetColumns(qr.Columns())

	return nil
}

func (panel *TablePanel) SetRows(rows [][]interface{}) {
	panel.Rows = rows
}

func (panel *TablePanel) SetColumns(columns []Column) {
	panel.Columns = columns
}

func (panel *TablePanel) SetSql(query string) {
	panel.RawSql = query
}

func (panel *TablePanel) SetTitle(title string) {
	panel.Title = title
}
