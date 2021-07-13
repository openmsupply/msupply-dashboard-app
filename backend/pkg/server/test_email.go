package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/simple-datasource-backend/pkg/auth"
	"github.com/grafana/simple-datasource-backend/pkg/emailer"
	"github.com/grafana/simple-datasource-backend/pkg/reportEmailer"
)

func (server *HttpServer) testEmail(rw http.ResponseWriter, request *http.Request) {

	vars := mux.Vars(request)
	id := vars["schedule-id"]

	emailConfig, err := auth.NewEmailConfig(server.db)
	if err != nil {
		log.DefaultLogger.Error("testEmail: auth.NewEmailConfig: ", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		panic(err)
	} else {
		log.DefaultLogger.Debug("testEmail: emailConfig:", emailConfig)
	}

	authConfig, err := auth.NewAuthConfig(server.db)
	if err != nil {
		log.DefaultLogger.Error("testEmail: auth.NewAuthConfig: ", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		panic(err)
	} else {
		log.DefaultLogger.Debug("testEmail: authConfig:", authConfig)
	}

	settings, err := server.db.GetSettings()
	if err != nil {
		log.DefaultLogger.Error("testEmail: db.GetSettings: ", err.Error())
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		panic(err)
	} else {
		log.DefaultLogger.Debug("testEmail: db.getSettings:", settings)
	}

	schedule, err := server.db.GetSchedule(id)
	if err != nil {
		log.DefaultLogger.Error("testData: server.db.GetSchedule: ", err.Error())
		http.Error(rw, err.Error(), http.StatusBadRequest)
		panic(err)
	} else {
		log.DefaultLogger.Debug("testEmail: schedule:", schedule)
	}

	em := emailer.New(emailConfig)
	re := reportEmailer.NewReportEmailer(server.db)
	re.CreateReport(*schedule, authConfig, settings.DatasourceID, *em)
	rw.WriteHeader(http.StatusOK)
}
