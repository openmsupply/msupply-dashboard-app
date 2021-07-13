package reportEmailer

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/bugsnag/bugsnag-go"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/simple-datasource-backend/pkg/api"
	"github.com/grafana/simple-datasource-backend/pkg/auth"
	"github.com/grafana/simple-datasource-backend/pkg/dbstore"
	"github.com/grafana/simple-datasource-backend/pkg/emailer"
	"github.com/grafana/simple-datasource-backend/pkg/reporter"
)

type ReportEmailer struct {
	sql        *dbstore.SQLiteDatasource
	inProgress bool
}

func NewReportEmailer(sql *dbstore.SQLiteDatasource) *ReportEmailer {
	return &ReportEmailer{sql: sql, inProgress: false}
}

func (re *ReportEmailer) configs() (*auth.AuthConfig, *auth.EmailConfig, int, error) {
	authConfig, err := auth.NewAuthConfig(re.sql)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.configs: NewAuthConfig: " + err.Error())
		return nil, nil, 0, err
	}

	emailConfig, err := auth.NewEmailConfig(re.sql)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.configs: NewEmailConfig: " + err.Error())
		return nil, nil, 0, err
	}

	settings, err := re.sql.GetSettings()
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.configs: GetSettings: " + err.Error())
		return nil, nil, 0, err
	}

	return authConfig, emailConfig, settings.DatasourceID, nil
}

func (re *ReportEmailer) cleanup(schedules []dbstore.Schedule) {
	log.DefaultLogger.Info("Starting Clean up...")
	for _, schedule := range schedules {

		log.DefaultLogger.Info(fmt.Sprintf("Deleting %s.xlsx...", schedule.Name))
		path := reporter.GetFilePath(schedule.Name)
		err := os.Remove(path)
		if err != nil {
			// Failure case shouldn't be much of a problem since we're using the schedule ID for the report name, at the moment
			// as it will just write to the same file and not create infinitely many if deleting always fails.
			log.DefaultLogger.Error(fmt.Sprintf("Could not delete %s... : %s.xlsx", schedule.Name, err.Error()))
		}

		schedule.UpdateNextReportTime()
		re.sql.UpdateSchedule(schedule.ID, schedule)
	}

	re.inProgress = false
}

func (re *ReportEmailer) CreateReport(schedule dbstore.Schedule, authConfig *auth.AuthConfig, datasourceID int, em emailer.Emailer) error {

	log.DefaultLogger.Debug("ReportEmailer.createReport: start")

	emails := make(map[string][]string)
	panels := make(map[string][]api.TablePanel)

	reportGroup, err := re.sql.ReportGroupFromSchedule(schedule)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReport: ReportGroupFromSchedule: " + err.Error())
		return err
	} else {
		log.DefaultLogger.Debug("ReportEmailer.createReport: ReportGroupFromSchedule:", reportGroup)
	}

	userIDs, err := re.sql.GroupMemberUserIDs(*reportGroup)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReport: GroupMemberUserIDs: " + err.Error())
		return err
	} else {
		log.DefaultLogger.Debug("ReportEmailer.createReport: GroupMemberUserIDs:", userIDs)
	}

	emailsFromUsers, err := api.GetEmails(*authConfig, userIDs, datasourceID)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReport: emailsFromUsers: " + err.Error())
		return err
	} else {
		log.DefaultLogger.Debug("ReportEmailer.createReport: GetEmails:", emailsFromUsers)
	}

	emails[schedule.ID] = emailsFromUsers

	reportContent, err := re.sql.GetReportContent(schedule.ID)
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReport: GetReportContent: " + err.Error())
		return err
	} else {
		log.DefaultLogger.Debug("ReportEmailer.createReport: GetReportContent:", reportContent)
	}

	panels[schedule.ID] = []api.TablePanel{}

	for _, content := range reportContent {
		interval := int64(schedule.Interval)
		to := strconv.FormatInt(time.Now().Unix(), 10)
		from := strconv.FormatInt(time.Now().Unix()-interval, 10)

		dashboard, err := api.NewDashboard(authConfig, content.DashboardID, from, to, datasourceID)

		if err != nil {
			log.DefaultLogger.Error("ReportEmailer.createReport: NewDashboard: " + err.Error())
			return err
		} else {
			log.DefaultLogger.Debug("ReportEmailer.createReport: NewDashboard:", dashboard)
		}

		panel := dashboard.Panel(content.PanelID)
		if panel != nil {
			panel.PrepSql(dashboard.Variables, content.Variables)
			panels[schedule.ID] = append(panels[schedule.ID], *panel)
		}
	}

	templatePath := reporter.GetFilePath("template")
	log.DefaultLogger.Debug("ReportEmailer.createReport: templatePath:", templatePath)
	reporter := reporter.NewReporter(templatePath)

	log.DefaultLogger.Debug("ReportEmailer.createReport: panels being used: ", panels)
	for scheduleID, reportSheetPanels := range panels {
		schedule, err := re.sql.GetSchedule(scheduleID)

		if err != nil {
			log.DefaultLogger.Error("ReportEmailer: GetSchedule: Could not create report to send.", err.Error())
			bugsnag.Notify(err)
		} else {
			log.DefaultLogger.Debug(fmt.Sprintf("ReportEmailer.createReport: schedule: %s", schedule.Name))
			report := reporter.CreateNewReport(scheduleID, schedule.Name)
			report.SetSheets(reportSheetPanels)
			err := report.Write(*authConfig)
			if err != nil {
				log.DefaultLogger.Error("ReportEmailer.createReports: report.Write: " + err.Error())
				return err
			}
		}
	}

	for scheduleID, recipientEmails := range emails {
		schedule, err := re.sql.GetSchedule(scheduleID)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer: GetSchedule: Could not create report to send.", err.Error())
			bugsnag.Notify(err)
		} else {
			attachmentPath := reporter.GetFilePath(schedule.Name)
			log.DefaultLogger.Debug("ReportEmailer.createReport: attachmentPath:", attachmentPath)
			em.BulkCreateAndSend(attachmentPath, recipientEmails, schedule.Name, schedule.Description)
		}
	}

	return nil
	// re.cleanup(schedules)
}

func (re *ReportEmailer) CreateReports() {
	log.DefaultLogger.Info("Creating Reports...")
	re.inProgress = true

	authConfig, emailConfig, datasourceID, err := re.configs()
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReports: re.configs: " + err.Error())
		return
	}

	em := emailer.New(emailConfig)

	schedules, err := re.sql.OverdueSchedules()
	if err != nil {
		log.DefaultLogger.Error("ReportEmailer.createReports: OverdueSchedules: " + err.Error())
		return
	}

	if len(schedules) > 0 {
		log.DefaultLogger.Info("Found schedules which are overdue...")
		for _, schedule := range schedules {
			log.DefaultLogger.Info(fmt.Sprintf("- %s : %s", schedule.Name, schedule.Description))
		}
	} else {
		log.DefaultLogger.Info("No schedules are overdue...")
		return
	}

	emails := make(map[string][]string)
	panels := make(map[string][]api.TablePanel)
	for _, schedule := range schedules {
		reportGroup, err := re.sql.ReportGroupFromSchedule(schedule)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer.createReports: ReportGroupFromSchedule: " + err.Error())
			return
		}

		userIDs, err := re.sql.GroupMemberUserIDs(*reportGroup)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer.createReports: GroupMemberUserIDs: " + err.Error())
			return
		}

		emailsFromUsers, err := api.GetEmails(*authConfig, userIDs, datasourceID)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer.createReports: emailsFromUsers: " + err.Error())
			return
		}

		emails[schedule.ID] = emailsFromUsers

		reportContent, err := re.sql.GetReportContent(schedule.ID)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer.createReports: GetReportContent: " + err.Error())
			return
		}

		panels[schedule.ID] = []api.TablePanel{}

		for _, content := range reportContent {
			interval := int64(schedule.Interval)
			to := strconv.FormatInt(time.Now().Unix(), 10)
			from := strconv.FormatInt(time.Now().Unix()-interval, 10)

			dashboard, err := api.NewDashboard(authConfig, content.DashboardID, from, to, datasourceID)

			if err != nil {
				log.DefaultLogger.Error("ReportEmailer.createReports: NewDashboard: " + err.Error())
				return
			}

			panel := dashboard.Panel(content.PanelID)

			if panel != nil {
				panel.PrepSql(dashboard.Variables, content.Variables)
				panels[schedule.ID] = append(panels[schedule.ID], *panel)
			}

		}
	}

	templatePath := reporter.GetFilePath("template")
	reporter := reporter.NewReporter(templatePath)

	for scheduleID, reportSheetPanels := range panels {
		schedule, err := re.sql.GetSchedule(scheduleID)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer: GetSchedule: Could not create report to send.", err.Error())
			bugsnag.Notify(err)
		} else {
			report := reporter.CreateNewReport(scheduleID, schedule.Name)

			report.SetSheets(reportSheetPanels)
			err := report.Write(*authConfig)
			if err != nil {
				log.DefaultLogger.Error("ReportEmailer.createReports: report.Write: " + err.Error())
				return
			}
		}

	}

	for scheduleID, recipientEmails := range emails {
		schedule, err := re.sql.GetSchedule(scheduleID)
		if err != nil {
			log.DefaultLogger.Error("ReportEmailer: GetSchedule: Could not create report to send.", err.Error())
			bugsnag.Notify(err)
		} else {
			attachmentPath := reporter.GetFilePath(schedule.Name)
			em.BulkCreateAndSend(attachmentPath, recipientEmails, schedule.Name, schedule.Description)
		}
	}

	re.cleanup(schedules)

}
