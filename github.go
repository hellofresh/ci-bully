package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type prType struct {
	client       *github.Client
	ctx          *context.Context
	owner        string
	repo         string
	inactiveDays int
	CloseOn      int
	GHpr         *github.PullRequest
}

const (
	HOURSINADAY = 24.0
)

func actions(currentPr prType) {
	var takeAction bool
	var actionTaken action

	for _, actionItem := range runConfig.Actions {
		switch {
		case currentPr.inactiveDays == actionItem.Day:
			actionTaken = actionItem
			takeAction = true
			break
		case currentPr.inactiveDays > actionItem.Day && actionItem.Last:
			// Last action
			actionTaken = actionItem
			takeAction = true
		}
	}
	if !takeAction {
		fmt.Printf("[skipping] \n")
		return
	}

	currentPr.CloseOn = remainingDaysUntilClose(currentPr)

	if arguments["--enable"].(bool) {
		commentOnPr(currentPr, actionTaken.Message)
		switch actionTaken.Action {
		case "close":
			closePr(currentPr)
		default:
			fmt.Printf("[%s]\n", actionTaken.Action)
		}
	} else {
		fmt.Printf("[%s 'DRY MODE']\n", actionTaken.Action)
	}
}

func remainingDaysUntilClose(pullRequest prType) int {
	for _, actionItem := range runConfig.Actions {
		if actionItem.Action == "close" {
			return actionItem.Day - pullRequest.inactiveDays
		}
	}
	return 0
}

func checkOpenPRs(ctx *context.Context, client *github.Client, owner string, repo string) {
	openPullRequests, _, err := client.PullRequests.List(*ctx, owner, repo, nil)
	if err != nil {
		fmt.Printf("Error %s\n", err)
		os.Exit(1)
	}

	for _, openPullRequest := range openPullRequests {
		lastDate, err := lastActionDate(ctx, client, owner, repo, openPullRequest)
		if err != nil {
			fmt.Printf("Error %s\n", err)
			os.Exit(1)
		}

		currentPr := prType{
			client:       client,
			ctx:          ctx,
			owner:        owner,
			repo:         repo,
			inactiveDays: daysSince(lastDate),
			GHpr:         openPullRequest,
		}
		fmt.Printf("Repo %s/%s/#%d user '%s' open since '%d' days : ", owner, repo, *openPullRequest.Number, *openPullRequest.User.Login, currentPr.inactiveDays)
		// take action if any
		actions(currentPr)
	}
}

func lastActionDate(ctx *context.Context, client *github.Client, owner string, repo string, pullRequest *github.PullRequest) (*time.Time, error) {
	if !runConfig.DaysSinceLastCommit {
		return pullRequest.CreatedAt, nil
	}
	return lastCommitDate(ctx, client, owner, repo, pullRequest)
}

func lastCommitDate(ctx *context.Context, client *github.Client, owner string, repo string, pullRequest *github.PullRequest) (*time.Time, error) {
	commitsList, _, err := client.PullRequests.ListCommits(*ctx, owner, repo, pullRequest.GetNumber(), nil)
	if err != nil {
		return nil, err
	}

	var lastDate time.Time
	for _, commit := range commitsList {
		lastDate = commit.Commit.Committer.GetDate()
	}

	return &lastDate, nil
}

func daysSince(date *time.Time) int {
	if !runConfig.OnlyWorkdays {
		return int(time.Since(*date).Hours() / HOURSINADAY)
	}

	return workdaysBetweenDates(*date, time.Now())
}

// workdaysBetweenDates calculates the workdays between two dates.
func workdaysBetweenDates(t1, t2 time.Time) int {
	if t2.Before(t1) {
		t1, t2 = t2, t1
	}

	days := 0
	for {
		if t1.After(t2) || t1.Equal(t2) {
			return days
		}
		if t1.Weekday() != time.Saturday && t1.Weekday() != time.Sunday {
			days++
		}

		t1 = t1.Add(time.Hour * HOURSINADAY)
	}
	return days
}

func loopOverRepos() {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: runConfig.Token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	for _, repo := range runConfig.Repos {
		owner := strings.Split(repo, "/")[0]
		repo := strings.Split(repo, "/")[1]
		checkOpenPRs(&ctx, client, owner, repo)
	}
}

func commentOnPr(pr prType, message string) {
	message = strings.Replace(message, "_USER_", *pr.GHpr.User.Login, -1)
	message = strings.Replace(message, "_SINCE_", strconv.Itoa(pr.inactiveDays), -1)
	message = strings.Replace(message, "_TILL_", strconv.Itoa(pr.CloseOn), -1)

	commentMsg := &github.IssueComment{Body: &message}
	_, _, err := pr.client.Issues.CreateComment(*pr.ctx, pr.owner, pr.repo, *pr.GHpr.Number, commentMsg)
	checkError(fmt.Sprintf("Error failed to comment on %s/%s #%d", pr.owner, pr.repo, *pr.GHpr.Number), err)
}

func closePr(pr prType) {
	*pr.GHpr.State = "closed"
	_, _, err := pr.client.PullRequests.Edit(*pr.ctx, pr.owner, pr.repo, *pr.GHpr.Number, pr.GHpr)
	checkError(fmt.Sprintf("Error failed to close PR %s/%s #%d", pr.owner, pr.repo, *pr.GHpr.Number), err)
}
