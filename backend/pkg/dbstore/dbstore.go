package dbstore

import (
	"context"
	"database/sql"
	"errors"

	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	_ "github.com/mattn/go-sqlite3"
)

// TODOs:
// SQL In separate files.
// Repository structs for each table? i.e. `ScheduleRepository` which has the methods `ScheduleRepository.update()` etc to partition the datasource struct.
// Common serialization/deserialization methods
// Better/consistent erroring
// Better/consistent return values

// Basic SQLite datasource
type SQLiteDatasource struct {
	instanceManager instancemgmt.InstanceManager
	Path            string
}

// Basic plugin instance settings
type InstanceSettings struct {
	httpClient *http.Client
}

type queryModel struct {
	Format string `json:"format"`
}

func GetDataSource() *SQLiteDatasource {
	log.DefaultLogger.Info("GetDatasource")

	instanceManager := datasource.NewInstanceManager(getDataSourceInstanceSettings)
	dataPath := filepath.Join("..", "data", "msupply.db")

	sqlDatasource := &SQLiteDatasource{
		instanceManager: instanceManager,
		Path:            dataPath,
	}

	log.DefaultLogger.Debug("mSupply App: DataPath=" + dataPath)
	sqlDatasource.Init()

	return sqlDatasource
}

func getDataSourceInstanceSettings(setting backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return &InstanceSettings{
		httpClient: &http.Client{},
	}, nil
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifer).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (datasource *SQLiteDatasource) QueryData(ctx context.Context, request *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	if os.Getenv("APP_ENV") != "production" {
		log.DefaultLogger.Info("QueryData", "request", request)
	}

	// TODO: Use context to timeout/cancel?
	response := backend.NewQueryDataResponse()

	for _, query := range request.Queries {
		res := datasource.query(ctx, query)
		response.Responses[query.RefID] = res
	}

	return response, nil
}

func (datasource *SQLiteDatasource) query(ctx context.Context, query backend.DataQuery) backend.DataResponse {
	var queryModel queryModel

	response := backend.DataResponse{}
	response.Error = json.Unmarshal(query.JSON, &queryModel)
	response.Error = errors.New("Queries are not supported!")
	if response.Error != nil {
		return response
	}

	if queryModel.Format == "" {
		log.DefaultLogger.Warn("format is empty. defaulting to time series")
	}

	frame := data.NewFrame("response")
	frame.Fields = append(frame.Fields,
		data.NewField("time", nil, []time.Time{query.TimeRange.From, query.TimeRange.To}),
	)
	frame.Fields = append(frame.Fields,
		data.NewField("values", nil, []int64{10, 20}),
	)
	response.Frames = append(response.Frames, frame)

	return response
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (datasource *SQLiteDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {

	err := datasource.Ping()

	var status backend.HealthStatus
	var message string
	if err != nil {
		status = backend.HealthStatusError
		message = "Could not ping the database: " + err.Error()
	} else {
		status = backend.HealthStatusOk
		message = "Yeah, nah, All good"
	}

	return &backend.CheckHealthResult{
		Status:  status,
		Message: message,
	}, nil
}

func (datasource *SQLiteDatasource) Ping() error {
	log.DefaultLogger.Info("Pinging Database")

	db, err := sql.Open("sqlite3", datasource.Path)
	defer db.Close()

	if err != nil {
		log.DefaultLogger.Error("Ping - sql.Open: ", err.Error())
		return err
	}

	err = db.Ping()
	if err != nil {
		log.DefaultLogger.Warn("Ping - sql.Ping: ", err.Error())
		return err
	}

	return nil
}

func (datasource *SQLiteDatasource) Init() {
	log.DefaultLogger.Info("Initializing Database")

	err := datasource.Ping()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Init - Ping. ", err.Error())
		panic(err)
	}

	db, err := sql.Open("sqlite3", datasource.Path)
	defer db.Close()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Init - sql.Open : ", err.Error())
		panic(err)
	}

	stmt, err := db.Prepare("CREATE TABLE IF NOT EXISTS Schedule (id TEXT PRIMARY KEY, interval INTEGER, nextReportTime INTEGER, name TEXT, description TEXT, lookback INTEGER, reportGroupID TEXT, time TEXT, day INTEGER, FOREIGN KEY(reportGroupID) REFERENCES ReportGroup(id))")
	stmt.Exec()
	defer stmt.Close()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Could not create Schedule:", err.Error())
		panic(err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS Config (id TEXT PRIMARY KEY, grafanaUsername TEXT, grafanaPassword TEXT, emailPassword TEXT, email TEXT, datasourceID INTEGER, emailHost TEXT, emailPort INTEGER, grafanaURL TEXT)")
	stmt.Exec()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Could not create Config:", err.Error())
		panic(err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS ReportGroup (id TEXT PRIMARY KEY, name TEXT, description TEXT)")
	stmt.Exec()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Could not create ReportGroup:", err.Error())
		panic(err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS ReportGroupMembership (id TEXT PRIMARY KEY, userID TEXT, reportGroupID TEXT, FOREIGN KEY(reportGroupID) REFERENCES ReportGroup(id))")
	stmt.Exec()

	if err != nil {
		log.DefaultLogger.Error("FATAL. Could not create ReportGroupMembership:", err.Error())
		panic(err)
	}

	stmt, err = db.Prepare("CREATE TABLE IF NOT EXISTS ReportContent (id TEXT PRIMARY KEY, scheduleID TEXT, panelID INTEGER, dashboardID TEXT, lookback INTEGER, variables TEXT, FOREIGN KEY(scheduleID) REFERENCES Schedule(id))")
	stmt.Exec()
	if err != nil {
		log.DefaultLogger.Error("FATAL. Could not create ReportContent:", err.Error())
		panic(err)
	}

	log.DefaultLogger.Info("Database initialized!")
}
