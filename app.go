// app.go

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type App struct {
	Router *mux.Router
	DB     *sql.DB
}

func (a *App) Initialize(user, password, dbname string) {
	connectionString := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable", user, password, dbname)
	var err error
	a.DB, err = sql.Open("postgres", connectionString)
	if err != nil {
		log.Fatal(err)
	}
	a.Router = mux.NewRouter()
	a.initializeRoutes()
}

func (a *App) Run(addr string) {
	log.Fatal(http.ListenAndServe(addr, a.Router))
}

func (a *App) initializeRoutes() {
	a.Router.HandleFunc("/issue", a.createIssue).Methods("POST")
	a.Router.HandleFunc("/error", a.createError).Methods("POST")
	a.Router.HandleFunc("/issue/jira", a.createIssueInJira).Methods("POST")
	a.Router.HandleFunc("/issue/status/{issue_jira_id:[a-zA-Z0-9]+}", a.getStatusIssue).Methods("GET")
	a.Router.HandleFunc("/job/{issue_jira_id:[a-zA-Z0-9]*}", a.getJob).Methods("GET")
	a.Router.HandleFunc("/issue/{issue_jira_id:[a-zA-Z0-9]*}", a.deleteIssue).Methods("DELETE")
	a.Router.HandleFunc("/issue/{issue_jira_id:[a-zA-Z0-9]*}", a.updateIssue).Methods("UPDATE")
	a.Router.HandleFunc("/issue", a.getIssue).Methods("GET")
	a.Router.HandleFunc("/issue/{issue_jira_id:[a-zA-Z0-9]*}", a.GetIssueByJiraID).Methods("GET")
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
}

func (a *App) createIssue(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	var i Issues
	decoder := json.NewDecoder(r.Body)
	fmt.Println("Decoding body request creating issue")
	if err := decoder.Decode(&i); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()

	if i.ErrorCode == "" {
		fmt.Println("Pushing issue to backlog Jira")
		if err := PushIssueToBacklogJira(); err != nil {
			respondWithError(w, http.StatusInternalServerError, "Unable to push issue to backlog of Jira")
		}
	} else {
		i.CreatedAt = time.Now()
		i.UpdatedAt = time.Now()
		if err := i.createIssue(a.DB); err != nil {
			fmt.Println("Creating issue")
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondWithJSON(w, http.StatusCreated, i)
		fmt.Println("Created issue successfully")
	}
}

func (a *App) createIssueInJira(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	var i IssueRequest
	decoder := json.NewDecoder(r.Body)
	fmt.Println("Decoding body request creating issue")
	if err := decoder.Decode(&i); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()
	vars := mux.Vars(r)
	errorCode := vars["errorCode"]
	content := vars["content"]
	var projectID string
	if strings.Contains(errorCode, "vm_") {
		projectID = "10000"
	} else if strings.Contains(errorCode, "db_") {
		projectID = "10002"
	} else if strings.Contains(errorCode, "k8s_") {
		projectID = "10001"
	} else if strings.Contains(errorCode, "api_") {
		projectID = "10003"
	} else {
		projectID = "10004"
	}
	err := PushIssueToProject(projectID, "10004", "xplat", "xplat", content)
	if err != nil {
		fmt.Printf("Unable to create issue in Jira: [%s]\n", err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	err1 := AddStepLog(a.DB, "10004", "xplat", "xplat", content, "to do", time.Now(), time.Now())
	if err1 != nil {
		fmt.Printf("Unable to add  step log to DB: [%s]\n", err.Error())
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
}

func PushIssueToBacklogJira() error {
	return nil
}

func PushIssueToProject(projectID, issueType, assignee, reporter, content string) error {
	url := fmt.Sprintf("10.0.0.10:8000/issue/?project_id=%s&issuetype=%s&assignee=%s&reporter=%s&content=%s&summary=%s&environment=environment", projectID, issueType, assignee, reporter, content, content)
	fmt.Println("url is: ", url)
	// url := `10.0.0.10:8000/issue/?project_id=` + projectID + `&issuetype=` + issueType + `&assignee=` + assignee + `&reporter=` + reporter + `&content=` + content + `&summary=` + content + `&environment=environment`
	method := "POST"
	payload := strings.NewReader(``)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return err
	}
	req.Header.Add("Content-type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("Unable to perform request: [%s]", err.Error())
		fmt.Println(err)
		return err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		fmt.Printf("Unable to read response body: [%s]", err.Error())
		return err
	}
	fmt.Println(string(body))
	return nil
}

func (a *App) createError(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	var e ErrorStore
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&e); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	defer r.Body.Close()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	if err := e.createError(a.DB); err != nil {
		fmt.Println("Creating error store")
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fmt.Println("Created error store successfully")
	respondWithJSON(w, http.StatusCreated, e)
}

func (a *App) getStatusIssue(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	issueJiraID := vars["issue_jira_id"]
	fmt.Println("issue_jira_id is: ", issueJiraID)
	issue := Issues{
		IssueJiraID: issueJiraID,
	}
	err := issue.getStatusIssue(a.DB, issueJiraID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, issue.Status)
}

func (a *App) getJob(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	issueJiraID := vars["issue_jira_id"]
	issue := Issues{
		IssueJiraID: issueJiraID,
	}
	issue.ApiJob(a.DB, issueJiraID)

	respondWithJSON(w, http.StatusOK, "")
}

func (a *App) deleteIssue(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	issueJiraID := vars["issue_jira_id"]
	issue := Issues{
		IssueJiraID: issueJiraID,
	}
	if err := issue.DeleteIssue(a.DB, issueJiraID); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, map[string]string{"delete": "success"})
}

func (a *App) updateIssue(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	issueJiraID := vars["issue_jira_id"]
	status := vars["status"]
	issue := Issues{
		IssueJiraID: issueJiraID,
	}
	if err := issue.UpdateIssueStatusInDB(a.DB, status); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, issue)
}

func (a *App) getIssue(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	issue, err := a.GetIssue(a.DB)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, issue)
}

func (a *App) GetIssueByJiraID(w http.ResponseWriter, r *http.Request) {
	enableCors(&w)
	vars := mux.Vars(r)
	issueJiraID := vars["issue_jira_id"]
	issue := Issues{
		IssueJiraID: issueJiraID,
	}
	if err := issue.GetIssueByJiraID(a.DB, issueJiraID); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondWithJSON(w, http.StatusOK, issue)
}
