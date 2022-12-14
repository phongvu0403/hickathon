// model.go
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type BaseModel struct {
	ID        int `json:"id"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Issues struct {
	BaseModel
	TenantID    string `json:"tenantId"`
	VpcID       string `json:"vpcId"`
	RegionID    string `json:"regionId"`
	IssueJiraID string `json:"issueJiraID"`
	Name        string `json:"name"`
	DataLog     string `json:"dataLog"`
	ErrorCode   string `json:"errorCode"`
	Status      string `json:"status"`
	Service     string `json:"service"`
}

type IssuesReturn struct {
	Issue Issues
	Logs  []LogIssueResponse
}

type StepLog struct {
	BaseModel
	ReporterName  string `json:"reporterName"`
	SupporterName string `json:"supporterName"`
	IssueID       string `json:"issueID"`
	Description   string `json:"description"`
	SupporterJira string `json:"supporterJira"`
	Status        string `json:"status"`
}

type Reporter struct {
	BaseModel
	Username string `json:"username"`
	Email    string `json:"email"`
}

type ErrorStore struct {
	BaseModel
	ErrorCode   string `json:"errorCode"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Service     string `json:"service"`
}

type IssueResponse struct {
	Expand string `json:"expand"`
	ID     string `json:"id"`
	Self   string `json:"self"`
	Key    string `json:"key"`
	Fields struct {
		Status struct {
			Self           string `json:"self"`
			Description    string `json:"description"`
			IconURL        string `json:"iconUrl"`
			Name           string `json:"name"`
			ID             string `json:"id"`
			StatusCategory struct {
				Self      string `json:"self"`
				ID        int    `json:"id"`
				Key       string `json:"key"`
				ColorName string `json:"colorName"`
				Name      string `json:"name"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

type IssueRequest struct {
	ErrorCode    string `json:"errorCode"`
	Content      string `json:"content"`
	ReporterName string `json:"reporterName"`
}

type ResponseJira struct {
	Id string `json:"id"`
}

type LogIssueResponse struct {
	Id            string `json:"id"`
	IssueId       string `json:"issueId"`
	Status        string `json:"status"`
	ReporterName  string `json:"reporterName"`
	SupporterName string `json:"supporterName"`
	Description   string `json:"description"`
}

func (issue *Issues) ApiJob(db *sql.DB, issueJiraID string) {
	for {
		status1 := issue.GetIssueStatusFromDB(db, issueJiraID)
		status2 := issue.GetJiraIssueStatus(db, issueJiraID)
		if status1 != status2 {
			if err := issue.UpdateIssueStatusInDB(db, status2); err != nil {
				fmt.Printf("Unable to update status of issue in DB")
			}
		}
		time.Sleep(30 * time.Second)
	}
}

func (issue *Issues) GetIssueStatusFromDB(db *sql.DB, issueJiraID string) string {
	var status string
	// db.QueryRow("SELECT status FROM issues WHERE issue_jira_id = ?", issueJiraID)
	err := db.QueryRow("SELECT status FROM issues WHERE issue_jira_id=$1", issueJiraID).Scan(status)
	if err != nil {
		fmt.Printf("Unable to query status with issue_jira_id from issues table")
	}
	return status
}

func (issue *Issues) GetJiraIssueStatus(db *sql.DB, issueJiraID string) string {
	url := fmt.Sprintf("http://10.0.0.4:8080/rest/api/2/issue/%s?fields=status", issueJiraID)
	method := "GET"
	var issueResponse IssueResponse

	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		fmt.Printf("Unable to create new request to get issue of Jira: [%s]", err.Error())
		return ""
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to perform request getting Issue Jira: [%s]", err.Error())
		return ""
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("Unable to read body of response from Jira: [%s]\n", err.Error())
		return ""
	}
	dataBody := []byte(body)
	err = json.Unmarshal(dataBody, &issueResponse)
	if err != nil {
		fmt.Printf("Unable to Unmarshal response body from jira: [%s]\n", err.Error())
	}

	return issueResponse.Fields.Status.Name
}

func (errorStore *ErrorStore) createError(db *sql.DB) error {
	err := db.QueryRow("INSERT INTO error_store(error_code, name, description, service, created_at, updated_at) VALUES($1, $2, $3, $4, $5, $6) RETURNING id",
		errorStore.ErrorCode, errorStore.Name, errorStore.Description, errorStore.Service, errorStore.CreatedAt, errorStore.UpdatedAt).Scan(&errorStore.ID)
	if err != nil {
		return err
	}
	return nil
}

func (issue *Issues) createIssue(db *sql.DB) error {
	err := db.QueryRow("INSERT INTO issues(tenant_id, vpc_id, region_id, issue_jira_id, name, data_log, error_code, status, service, created_at, updated_at) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id",
		issue.TenantID, issue.VpcID, issue.RegionID, issue.IssueJiraID, issue.Name, issue.DataLog, issue.ErrorCode, issue.Status, issue.Service, issue.CreatedAt, issue.UpdatedAt).Scan(&issue.ID)
	if err != nil {
		return err
	}
	return nil
}

func (issue *Issues) getStatusIssue(db *sql.DB, issueJiraID string) error {
	// var status string
	err := db.QueryRow("SELECT status FROM issues WHERE issue_jira_id=$1", issueJiraID).Scan(&issue.Status)
	if err != nil {
		fmt.Printf("Unable to query status with issue_jira_id from issues table")
		return fmt.Errorf("Unable to query status with issue_jira_id from issues table: [%s]", err.Error())
	}
	return err
}

func (issue *Issues) UpdateIssueStatusInDB(db *sql.DB, status string) error {
	_, err := db.Exec("UPDATE issues SET status=$1", status)
	return err
}

func (app *App) UpdateIssueJiraIdInDB(db *sql.DB, issueJiraId string) error {
	_, err := db.Exec("UPDATE issues SET issue_jira_id=$1", issueJiraId)
	return err
}

func (issue *Issues) DeleteIssue(db *sql.DB, issueJiraID string) error {
	_, err := db.Exec("DELETE FROM issues WHERE issue_jira_id=$1", issueJiraID)
	return err
}

// func (app *App) GetIssue(db *sql.DB) (sql.Result, error) {
func (app *App) GetIssue(db *sql.DB) ([]Issues, error) {
	rows, err := db.Query("SELECT id, tenant_id, vpc_id, region_id, issue_jira_id, name, data_log, error_code, status, service, created_at, updated_at FROM issues")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []Issues{}
	for rows.Next() {
		var i Issues
		if err := rows.Scan(&i.ID, &i.TenantID, &i.VpcID, &i.RegionID, &i.IssueJiraID, &i.Name, &i.DataLog, &i.ErrorCode,
			&i.Status, &i.Service, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		issues = append(issues, i)
	}
	// issue, err := db.Exec("SELECT * FROM issues")
	// return issue, err
	return issues, nil
}

func (issue *Issues) GetIssueByJiraID(db *sql.DB, issueJiraID string) error {
	return db.QueryRow("SELECT id, tenant_id, vpc_id, region_id, name, data_log, error_code, status, service, created_at, updated_at FROM issues WHERE issue_jira_id=$1",
		issueJiraID).Scan(&issue.ID, &issue.TenantID, &issue.VpcID, &issue.RegionID, &issue.Name, &issue.DataLog, &issue.ErrorCode,
		&issue.Status, &issue.Service, &issue.CreatedAt, &issue.UpdatedAt)
}

func AddStepLog(db *sql.DB, IssueID, reporterName, supporterName, description, status string, createdAt, updatedAt time.Time) error {
	var stepLog StepLog
	err := db.QueryRow("INSERT INTO step_log(issue_id, reporter_name, supporter_name, description, status, created_at, updated_at) VALUES($1, $2, $3, $4, $5, $6, $7) RETURNING id",
		IssueID, reporterName, supporterName, description, status, createdAt, updatedAt).Scan(&stepLog.ID)
	if err != nil {
		return err
	}
	return nil
}

func (app *App) GetLogsByIssueJiraId(db *sql.DB, issueJiraID string) ([]LogIssueResponse, error) {
	textQ := "SELECT id, issue_id, reporter_name, supporter_name, description, status FROM step_log WHERE issue_id='" + string(issueJiraID) + "'"
	fmt.Print(textQ)
	rows, err := db.Query(textQ)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []LogIssueResponse{}

	count := 0

	for rows.Next() {
		count++
		var issue LogIssueResponse
		if err := rows.Scan(&issue.Id, &issue.IssueId, &issue.ReporterName, &issue.SupporterName, &issue.Description, &issue.Status); err != nil {
			return nil, err
		}
		logs = append(logs, issue)
	}

	fmt.Print("count", count)

	// issue, err := db.Exec("SELECT * FROM issues")
	// return issue, err
	return logs, nil
}
