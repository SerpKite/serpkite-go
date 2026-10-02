// Command search runs a SerpKite web search and prints the top results.
//
//	export SERPKITE_API_KEY=skt_live_...
//	go run ./examples/search "best espresso machine"
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	serpkite "github.com/serpkite/serpkite-go"
)

func main() {
	q := "best espresso machine"
	if len(os.Args) > 1 {
		q = strings.Join(os.Args[1:], " ")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c := serpkite.NewClient() // reads SERPKITE_API_KEY

	var info serpkite.ResponseInfo
	res, err := c.Search(ctx, serpkite.SearchParams{Q: q, Country: "us"}, serpkite.CaptureResponse(&info))
	if err != nil {
		var apiErr *serpkite.Error
		if errors.As(err, &apiErr) {
			log.Fatalf("%s (%d): %s [request %s]", apiErr.Code, apiErr.Status, apiErr.Message, apiErr.RequestID)
		}
		log.Fatal(err)
	}

	if ab := res.AnswerBox; ab != nil && ab.Answer != "" {
		fmt.Printf("Answer: %s\n\n", ab.Answer)
	}
	for _, r := range res.Results {
		fmt.Printf("%2d. %s\n    %s\n", r.Position, r.Title, r.Link)
	}
	fmt.Printf("\ncredits used: %v", res.Meta.CreditsUsed)
	if info.CreditsRemaining != nil {
		fmt.Printf(", remaining: %v", *info.CreditsRemaining)
	}
	fmt.Println()
}
