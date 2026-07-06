package restapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-task

type TaskInfo struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

type Task struct {
	Name      string         `json:"name"`
	Metadata  map[string]any `json:"metadata"`
	BeginTime string         `json:"begin_time"`
	EndTime   string         `json:"end_time,omitempty"`
	Duration  float64        `json:"duration,omitempty"`
	Progress  int            `json:"progress"`
	Success   bool           `json:"success"`
	RetValue  any            `json:"ret_value"`
	Exception any            `json:"exception"`
}

type TaskList struct {
	ExecutingTasks []Task `json:"executing_tasks"`
	FinishedTasks  []Task `json:"finished_tasks"`
}

func (c *Client) GetTasks(ctx context.Context, nameFilter string) (*TaskList, error) {
	endpoint := c.endpoint.JoinPath("/api/task")
	if nameFilter != "" {
		query := url.Values{}
		query.Add("name", nameFilter)
		endpoint.RawQuery = query.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	if httpResp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	tflog.Trace(ctx, "Ceph API response body", map[string]any{
		"response_body": string(body),
		"status_code":   httpResp.StatusCode,
	})

	var taskList TaskList
	err = json.Unmarshal(body, &taskList)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &taskList, nil
}

func decodeTaskInfo(ctx context.Context, body []byte, operation string) (*TaskInfo, error) {
	var taskInfo TaskInfo
	if err := json.Unmarshal(body, &taskInfo); err != nil {
		return nil, fmt.Errorf("unable to decode task response: %w", err)
	}
	tflog.Debug(ctx, operation+" returned 202, task is running", map[string]any{
		"task_name": taskInfo.Name,
		"metadata":  taskInfo.Metadata,
	})
	return &taskInfo, nil
}

func (c *Client) WaitForTask(ctx context.Context, taskInfo *TaskInfo) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// The dashboard keeps finished tasks around, and identity is only
	// (name, metadata), which repeated operations share (e.g. two resizes
	// of the same image). Once the running task shows up in the executing
	// list, its begin_time pins the run being waited for; until then, the
	// newest finished match is taken.
	var beginTime string

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("task '%s' did not complete within timeout: %w", taskInfo.Name, ctx.Err())
		case <-ticker.C:
			tasks, err := c.GetTasks(ctx, taskInfo.Name)
			if err != nil {
				return fmt.Errorf("unable to poll task '%s': %w", taskInfo.Name, err)
			}

			for _, task := range tasks.ExecutingTasks {
				tflog.Trace(ctx, "Executing task", map[string]interface{}{
					"name":       task.Name,
					"metadata":   task.Metadata,
					"progress":   task.Progress,
					"begin_time": task.BeginTime,
				})
			}

			for _, task := range tasks.FinishedTasks {
				tflog.Trace(ctx, "Finished task", map[string]interface{}{
					"name":     task.Name,
					"metadata": task.Metadata,
					"success":  task.Success,
					"duration": task.Duration,
					"end_time": task.EndTime,
				})
			}

			tflog.Debug(ctx, "Polling tasks", map[string]interface{}{
				"executing_task_count": len(tasks.ExecutingTasks),
				"finished_task_count":  len(tasks.FinishedTasks),
				"target_task_name":     taskInfo.Name,
				"target_task_metadata": taskInfo.Metadata,
			})

			var taskProgress *int
			for _, task := range tasks.ExecutingTasks {
				if task.Name == taskInfo.Name && reflect.DeepEqual(task.Metadata, taskInfo.Metadata) {
					if beginTime == "" {
						beginTime = task.BeginTime
					}
					taskProgress = &task.Progress
					break
				}
			}

			var finished *Task
			for i, task := range tasks.FinishedTasks {
				if task.Name != taskInfo.Name || !reflect.DeepEqual(task.Metadata, taskInfo.Metadata) {
					continue
				}
				if beginTime != "" {
					if task.BeginTime == beginTime {
						finished = &tasks.FinishedTasks[i]
						break
					}
					continue
				}
				if finished == nil || task.BeginTime > finished.BeginTime {
					finished = &tasks.FinishedTasks[i]
				}
			}

			if finished != nil {
				if !finished.Success {
					tflog.Error(ctx, "Task failed", map[string]interface{}{
						"task_name": taskInfo.Name,
						"metadata":  taskInfo.Metadata,
						"exception": finished.Exception,
						"duration":  finished.Duration,
					})
					return fmt.Errorf("task '%s' failed: %v", taskInfo.Name, finished.Exception)
				}

				tflog.Debug(ctx, "Task completed successfully", map[string]interface{}{
					"task_name": taskInfo.Name,
					"metadata":  taskInfo.Metadata,
					"duration":  finished.Duration,
				})
				return nil
			}

			logFields := map[string]interface{}{
				"task_name": taskInfo.Name,
				"metadata":  taskInfo.Metadata,
			}
			if taskProgress != nil {
				logFields["progress"] = *taskProgress
			}
			tflog.Debug(ctx, "Task still executing", logFields)
		}
	}
}
