package workers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type stats struct {
	Processed int         `json:"processed"`
	Failed    int         `json:"failed"`
	Jobs      interface{} `json:"jobs"`
	Enqueued  interface{} `json:"enqueued"`
	Retries   int64       `json:"retries"`
}

// Stats writes stats on response writer
func Stats(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := getStats()

	body, _ := json.MarshalIndent(stats, "", "  ")
	fmt.Fprintln(w, string(body))
}

// WorkerStats holds workers stats
type WorkerStats struct {
	Processed int               `json:"processed"`
	Failed    int               `json:"failed"`
	Enqueued  map[string]string `json:"enqueued"`
	Retries   int64             `json:"retries"`
}

// GetStats returns workers stats
func GetStats() *WorkerStats {
	_stats := getStats()
	enqueued := map[string]string{}
	if statsEnqueued, ok := _stats.Enqueued.(map[string]string); ok {
		enqueued = statsEnqueued
	}

	return &WorkerStats{
		Processed: _stats.Processed,
		Failed:    _stats.Failed,
		Retries:   _stats.Retries,
		Enqueued:  enqueued,
	}
}

func getStats() stats {
	jobs := make(map[string][]*map[string]interface{})
	enqueued := make(map[string]string)

	for _, m := range managers {
		queue := m.queueName()
		jobs[queue] = make([]*map[string]interface{}, 0)
		enqueued[queue] = ""
		for _, worker := range m.workers {
			message := worker.currentMsg
			startedAt := worker.startedAt

			if message != nil && startedAt > 0 {
				jobs[queue] = append(jobs[queue], &map[string]interface{}{
					"message":    message,
					"started_at": startedAt,
				})
			}
		}
	}

	_stats := stats{
		0,
		0,
		jobs,
		enqueued,
		0,
	}

	conn := Config.Pool.Get()
	defer conn.Close()

	err := conn.Send("multi")
	if err != nil {
		Logger.Println("unable to connect for multi: ", err)
	}
	err = conn.Send("get", Config.Namespace+"stat:processed")
	if err != nil {
		Logger.Println("could not get stats:processed: ", err)
	}
	err = conn.Send("get", Config.Namespace+"stat:failed")
	if err != nil {
		Logger.Println("could not get stats:failed: ", err)
	}
	err = conn.Send("zcard", Config.Namespace+Config.RetryKey)
	if err != nil {
		Logger.Println("could not send zcard: ", err)
	}

	for key := range enqueued {
		err = conn.Send("llen", fmt.Sprintf("%squeue:%s", Config.Namespace, key))
		if err != nil {
			Logger.Println("could not call llen: ", err)
		}
	}

	r, err := conn.Do("exec")

	if err != nil {
		Logger.Println("failed to retrieve stats:", err)
	}

	results := r.([]interface{})
	if len(results) == (3 + len(enqueued)) {
		for index, result := range results {
			if index == 0 && result != nil {
				_stats.Processed, _ = strconv.Atoi(string(result.([]byte)))
				continue
			}
			if index == 1 && result != nil {
				_stats.Failed, _ = strconv.Atoi(string(result.([]byte)))
				continue
			}

			if index == 2 && result != nil {
				_stats.Retries = result.(int64)
				continue
			}

			queueIndex := 0
			for key := range enqueued {
				if queueIndex == (index - 3) {
					enqueued[key] = fmt.Sprintf("%d", result.(int64))
				}
				queueIndex++
			}
		}
	}

	return _stats
}
