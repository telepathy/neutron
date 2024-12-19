package main

import (
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"net/http"
)

func main() {
	r := gin.Default()
	r.POST("/webhook", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		bodyString := string(body)
		log.Println(bodyString)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	_ = r.Run(":8888")
}
