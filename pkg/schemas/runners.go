package schemas

import (
	"hash/crc32"
	"strconv"
	"strings" // For string manipulation operations
	"time"

	goGitlab "gitlab.com/gitlab-org/api/client-go" // GitLab API client
)

type RunnerKey int

type RunnersList struct {
	Id                 int         `json:"id"`
	Description        string      `json:"description"`
	IpAddress          interface{} `json:"ip_address"`
	Active             bool        `json:"active"`
	Paused             bool        `json:"paused"`
	IsShared           bool        `json:"is_shared"`
	RunnerType         string      `json:"runner_type"`
	Name               string      `json:"name"`
	Online             bool        `json:"online"`
	CreatedAt          time.Time   `json:"created_at"`
	Status             string      `json:"status"`
	JobExecutionStatus string      `json:"job_execution_status"`
}

type Runner struct {
	Id          int         `json:"id"`
	Description string      `json:"description"`
	IpAddress   interface{} `json:"ip_address"`
	Active      bool        `json:"active"`
	Paused      bool        `json:"paused"`
	IsShared    bool        `json:"is_shared"`
	RunnerType  string      `json:"runner_type"`
	Name        interface{} `json:"name"`
	Online      bool        `json:"online"`
	CreatedBy   struct {
		Id          int    `json:"id"`
		Username    string `json:"username"`
		PublicEmail string `json:"public_email"`
		Name        string `json:"name"`
		State       string `json:"state"`
		Locked      bool   `json:"locked"`
		AvatarUrl   string `json:"avatar_url"`
		WebUrl      string `json:"web_url"`
	} `json:"created_by"`
	CreatedAt          time.Time     `json:"created_at"`
	Status             string        `json:"status"`
	JobExecutionStatus string        `json:"job_execution_status"`
	TagList            []string      `json:"tag_list"`
	RunUntagged        bool          `json:"run_untagged"`
	Locked             bool          `json:"locked"`
	MaximumTimeout     interface{}   `json:"maximum_timeout"`
	AccessLevel        string        `json:"access_level"`
	Version            string        `json:"version"`
	Revision           string        `json:"revision"`
	Platform           string        `json:"platform"`
	Architecture       string        `json:"architecture"`
	ContactedAt        time.Time     `json:"contacted_at"`
	MaintenanceNote    string        `json:"maintenance_note"`
	Projects           []interface{} `json:"projects"`
	Groups             []struct {
		Id     int    `json:"id"`
		WebUrl string `json:"web_url"`
		Name   string `json:"name"`
	} `json:"groups"`
}

type Runners map[RunnerKey]Runner

func (r Runner) Key() RunnerKey {
	return RunnerKey(strconv.Itoa(int(crc32.ChecksumIEEE([]byte(r.Name)))))
}
