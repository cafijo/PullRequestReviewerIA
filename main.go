package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/go-github/v51/github"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2"
	"os"
	"strings"
)

func main() {
	reviewResult := map[string][]string{}
	githubtoken := flag.String("github_token", "", "Your Git Hub Token")
	openaiAPIKey := flag.String("openai_api_key", "", "Your OpenAI API Key")
	githubPRID := flag.Int("github_pr_id", 0, "Your Github PR ID")
	flag.Parse()
	// Authenticating with the OpenAI API
	openaiClient := openai.NewClient(*openaiAPIKey)
	ctx := context.Background()
	// Authenticating with the Github API providing the token
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *githubtoken},
	)
	tc := oauth2.NewClient(ctx, ts)
	clientGithub := github.NewClient(tc)
	repoAndOwner := os.Getenv("GITHUB_REPOSITORY")
	owner := repoAndOwner[:strings.Index(repoAndOwner, "/")]
	repoName := repoAndOwner[strings.Index(repoAndOwner, "/")+1:]

	repo, _, err := clientGithub.Repositories.Get(ctx, owner, repoName)
	if err != nil {
		fmt.Printf("Error getting repository: %v", err)
		return
	}

	// Loop through the commits in the pull request
	files, _, err := clientGithub.PullRequests.ListFiles(ctx, repo.Owner.GetLogin(), repo.GetName(), *githubPRID, nil)
	if err != nil {
		fmt.Printf("Error getting commits: %v", err)
		return
	}
	for _, file := range files {
		// Getting the file name and content
		filename := file.GetFilename()
		rawURL := file.GetContentsURL()
		req, err := clientGithub.NewRequest("GET", rawURL, nil)
		if err != nil {
			fmt.Printf("Error getting commits: %v", err)
			continue
		}
		var fileContent *github.RepositoryContent
		var rawJSON json.RawMessage
		_, err = clientGithub.Do(ctx, req, &rawJSON)
		if err != nil {
			fmt.Printf("Error getting commits: %v", err)
			continue
		}

		fileUnmarshalError := json.Unmarshal(rawJSON, &fileContent)
		if fileUnmarshalError != nil {
			fmt.Printf("Error getting commits: %v", err)
			continue
		}

		content, err := fileContent.GetContent()
		if err != nil {
			fmt.Printf("Error getting commits: %v", err)
			continue
		}

		responseChatgpt, err := openaiClient.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:       openai.GPT3Dot5Turbo,
				Temperature: 0.5,
				Messages: []openai.ChatCompletionMessage{
					{
						Role:    openai.ChatMessageRoleUser,
						Content: fmt.Sprintf("Analise and find bugs on the code like sonar from next file in: %s", string(content)),
					},
				},
			},
		)

		if err != nil {
			fmt.Printf("ChatCompletion error: %v\n", err)
			continue
		}

		fmt.Println(responseChatgpt.Choices[0].Message.Content)

		reviewResult[filename] = append(reviewResult[filename], responseChatgpt.Choices[0].Message.Content)
	}

	reviews, _, err := clientGithub.PullRequests.ListReviews(ctx, repo.Owner.GetLogin(), repo.GetName(), *githubPRID, nil)
	if err != nil {
		fmt.Printf("Error getting comments: %v", err)
		return
	}
	if len(reviews) > 0 {
		// Delete previous comments
		for _, review := range reviews {
			if strings.Contains(review.GetBody(), "pr-review-actions[bot]") {
				_, _, err := clientGithub.PullRequests.DeletePendingReview(ctx, repo.Owner.GetLogin(), repo.GetName(), *githubPRID, review.GetID())
				if err != nil {
					fmt.Printf("Error deleting comment: %v", err)
					return
				}
			}
		}
	}

	for k, v := range reviewResult {
		// Create a new comment
		review := &github.PullRequestReviewRequest{
			CommitID: github.String(os.Getenv("GITHUB_SHA")),
			Body:     github.String(fmt.Sprintf("Automatic Commented Review by pr-review-actions[bot]. \n\n\nReview result for file \"%s\": \n\n %s", k, v)),
			NodeID:   github.String("pr-review-actions[bot]"),
		}
		_, _, err := clientGithub.PullRequests.CreateReview(ctx, repo.Owner.GetLogin(), repo.GetName(), *githubPRID, review)
		if err != nil {
			fmt.Printf("Error creating comment: %v", err)
			continue
		}
	}
	fmt.Printf("Review result: %v", reviewResult)
}
