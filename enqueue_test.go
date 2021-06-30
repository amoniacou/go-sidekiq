package workers

import (
	"encoding/json"

	"github.com/customerio/gospec"
	. "github.com/customerio/gospec"
	"github.com/gomodule/redigo/redis"
)

func EnqueueSpec(c gospec.Context) {
	was := Config.Namespace
	Config.Namespace = "prod:"

	c.Specify("Enqueue", func() {
		conn := Config.Pool.Get()
		defer conn.Close()

		c.Specify("makes the queue available", func() {
			_, err := Enqueue("enqueue1", "Add", []int{1, 2})
			c.Expect(err, Equals, nil)

			found, _ := redis.Bool(conn.Do("sismember", "prod:queues", "enqueue1"))
			c.Expect(found, IsTrue)
		})

		c.Specify("adds a job to the queue", func() {
			nb, _ := redis.Int(conn.Do("llen", "prod:queue:enqueue2"))
			c.Expect(nb, Equals, 0)

			_, err := Enqueue("enqueue2", "Add", []int{1, 2})
			c.Expect(err, Equals, nil)

			nb, _ = redis.Int(conn.Do("llen", "prod:queue:enqueue2"))
			c.Expect(nb, Equals, 1)
		})

		c.Specify("saves the arguments", func() {
			_, err := Enqueue("enqueue3", "Compare", []string{"foo", "bar"})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue3"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			args := result["args"].([]interface{})
			c.Expect(len(args), Equals, 2)
			c.Expect(args[0], Equals, "foo")
			c.Expect(args[1], Equals, "bar")
		})

		c.Specify("has a jid", func() {
			_, err := Enqueue("enqueue4", "Compare", []string{"foo", "bar"})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue4"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			jid := result["jid"].(string)
			c.Expect(len(jid), Equals, 24)
		})

		c.Specify("has enqueued_at that is close to now", func() {
			_, err := Enqueue("enqueue5", "Compare", []string{"foo", "bar"})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue5"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			ea := result["enqueued_at"].(float64)
			c.Expect(ea, Not(Equals), 0)
			c.Expect(ea, IsWithin(0.1), nowToSecondsWithNanoPrecision())
		})

		c.Specify("has retry and retry_max when set", func() {
			_, err := EnqueueWithOptions("enqueue6", "Compare", []string{"foo", "bar"}, EnqueueOptions{RetryMax: 13, Retry: true})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue6"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			retry := result["retry"].(bool)
			c.Expect(retry, Equals, true)

			retryMax := int(result["retry_max"].(float64))
			c.Expect(retryMax, Equals, 13)
		})

		c.Specify("sets Retry correctly when no count given", func() {
			_, err := EnqueueWithOptions("enqueue6", "Compare", []string{"foo", "bar"}, EnqueueOptions{Retry: true})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue6"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			retry := result["retry"].(bool)
			c.Expect(retry, Equals, true)
		})

		c.Specify("has retry_options when set", func() {
			_, err := EnqueueWithOptions(
				"enqueue7", "Compare", []string{"foo", "bar"},
				EnqueueOptions{
					RetryMax: 13,
					Retry:    true,
					RetryOptions: RetryOptions{
						Exp:      2,
						MinDelay: 0,
						MaxDelay: 60,
						MaxRand:  30,
					},
				})
			c.Expect(err, Equals, nil)

			bytes, _ := redis.Bytes(conn.Do("lpop", "prod:queue:enqueue7"))
			var result map[string]interface{}
			err = json.Unmarshal(bytes, &result)
			c.Expect(err, Equals, nil)
			c.Expect(result["class"], Equals, "Compare")

			retryOptions := result["retry_options"].(map[string]interface{})
			c.Expect(len(retryOptions), Equals, 4)
			c.Expect(retryOptions["exp"].(float64), Equals, float64(2))
			c.Expect(retryOptions["min_delay"].(float64), Equals, float64(0))
			c.Expect(retryOptions["max_delay"].(float64), Equals, float64(60))
			c.Expect(retryOptions["max_rand"].(float64), Equals, float64(30))
		})
	})

	c.Specify("EnqueueIn", func() {
		scheduleQueue := "prod:" + Config.ScheduleKey
		conn := Config.Pool.Get()
		defer conn.Close()

		c.Specify("has added a job in the scheduled queue", func() {
			_, err := EnqueueIn("enqueuein1", "Compare", 10, map[string]interface{}{"foo": "bar"})
			c.Expect(err, Equals, nil)

			scheduledCount, _ := redis.Int(conn.Do("zcard", scheduleQueue))
			c.Expect(scheduledCount, Equals, 1)

			_, err = conn.Do("del", scheduleQueue)
			c.Expect(err, Equals, nil)
		})

		c.Specify("has the correct 'queue'", func() {
			_, err := EnqueueIn("enqueuein2", "Compare", 10, map[string]interface{}{"foo": "bar"})
			c.Expect(err, Equals, nil)

			var data EnqueueData
			elem, err := conn.Do("zrange", scheduleQueue, 0, -1)
			c.Expect(err, Equals, nil)
			bytes, err := redis.Bytes(elem.([]interface{})[0], err)
			c.Expect(err, Equals, nil)
			err = json.Unmarshal(bytes, &data)
			c.Expect(err, Equals, nil)

			c.Expect(data.Queue, Equals, "enqueuein2")

			_, err = conn.Do("del", scheduleQueue)
			c.Expect(err, Equals, nil)
		})
	})

	Config.Namespace = was
}
