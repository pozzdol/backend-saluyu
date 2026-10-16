package handler

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimit allows `limit` requests per client IP per `window` (fixed window).
// ponytail: in-memory, one process only. Move to Redis when running more than one instance.
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	type entry struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	hits := map[string]*entry{}

	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()

		mu.Lock()
		if len(hits) > 10_000 { // drop expired entries so the map cannot grow forever
			for k, e := range hits {
				if now.After(e.reset) {
					delete(hits, k)
				}
			}
		}
		e, ok := hits[key]
		if !ok || now.After(e.reset) {
			e = &entry{reset: now.Add(window)}
			hits[key] = e
		}
		e.count++
		over, retry := e.count > limit, e.reset.Sub(now)
		mu.Unlock()

		if over {
			c.Header("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
			return
		}
		c.Next()
	}
}
