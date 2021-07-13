package api

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type Query struct {
	RefID         string `json:"refId"`
	IntervalMs    int    `json:"intervalMs"`
	MaxDataPoints int    `json:"maxDataPoints"`
	DatasourceID  int    `json:"datasourceId"`
	RawSQL        string `json:"rawSql"`
	Format        string `json:"format"`
}

type QueryRequest struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Queries []Query `json:"queries"`
}

func NewQuery(rawSql string, datasource int) *Query {
	return &Query{RawSQL: rawSql, DatasourceID: datasource, Format: "table", RefID: "A"}
}

func NewQueryRequest(rawSql string, from string, to string, datasourceID int) *QueryRequest {
	query := NewQuery(rawSql, datasourceID)
	queryRequest := &QueryRequest{From: from, To: to, Queries: []Query{*query}}
	return queryRequest
}

func (qr *QueryRequest) ToRequestBody() (*strings.Reader, error) {
	parsed, err := json.Marshal(qr)
	if err != nil {
		log.DefaultLogger.Error("ToRequestBody: json.Marshal: " + err.Error())
		return nil, err
	}

	body := strings.NewReader(string(parsed))
	return body, nil
}

type QueryResponse struct {
	Results struct {
		A struct {
			RefID string `json:"refId"`
			Meta  struct {
				ExecutedQueryString string `json:"executedQueryString"`
				RowCount            int    `json:"rowCount"`
			} `json:"meta"`
			Series interface{} `json:"series"`
			Tables []struct {
				Columns []Column        `json:"columns"`
				Rows    [][]interface{} `json:"rows"`
			} `json:"tables"`
			Dataframes interface{} `json:"dataframes"`
		} `json:"A"`
	} `json:"results"`
}

func NewQueryResponse(response *http.Response) (*QueryResponse, error) {
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.DefaultLogger.Error("NewQueryResponse: ioutil.ReadAll: " + err.Error())
		return nil, err
	}
	log.DefaultLogger.Debug(fmt.Sprintf("NewQueryResponse: body: %s", body));

	var qr QueryResponse
	err = json.Unmarshal(body, &qr)
	if err != nil {
		log.DefaultLogger.Error("NewQueryResponse: json.Unmarshal: " + err.Error())
		return nil, err
	}

	return &qr, nil
}

func (qr *QueryResponse) Rows() [][]interface{} {

	if len(qr.Results.A.Tables) > 0 {
		return qr.Results.A.Tables[0].Rows
	}

	return nil
}

func (qr *QueryResponse) Columns() []Column {
	if len(qr.Results.A.Tables) > 0 {
		return qr.Results.A.Tables[0].Columns
	}

	return nil
}
