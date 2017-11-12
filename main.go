package main

import (
    "github.com/gin-gonic/gin"
    "fmt"
    log "github.com/sirupsen/logrus"
    "strings"
    "net/url"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "time"
    "os"
)

type ApiResponse struct {
    Code    int         `json:"code"`
    Message string      `json:"message"`
    Data    interface{} `json:"data"`
}

type httpLableQuery struct {
    Host string `form:"host"`
    HostParse struct {
        User string
        Repo string
    }
    Show string `form:"show"`
}

type GitHubRepos struct {
    WatchersCount   int `json:"subscribers_count"`
    StargazersCount int `json:"stargazers_count"`
    ForksCount      int `json:"forks_count"`
}

type GitHubReposCache struct {
    LastUpdate  time.Time
    GitHubRepos *GitHubRepos
}

var GitHubReposCacheList = make(map[string]*GitHubReposCache)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        log.Fatal("$PORT must be set")
    }

    log.SetLevel(log.DebugLevel)

    router := gin.Default()

    router.GET("/ping", func(c *gin.Context) {
        c.JSON(200, ApiResponse{
            Code:    200,
            Message: "pong",
        })
    })

    router.GET("/lable", httpLable)

    router.NoRoute(func(c *gin.Context) {
        c.JSON(404, ApiResponse{
            Code:    404,
            Message: "Not found",
        })
    })

    if err := router.Run(":"+port); err != nil {
        log.Panicf("ERROR! HttpAPI init: %s", err)
    }
}

func httpLable(ctx *gin.Context) {
    log.Debugf("httpLable | Context = %+v", ctx)

    httpLableQuery := httpLableQuery{}
    err := httpLableQuery.ParseByCTX(ctx)
    if err != nil {
        return
    }

    log.Debugf("httpLable | httpLableQuery = %+v", httpLableQuery)

    GitHubReposCacheKey := fmt.Sprintf("%s/%s", httpLableQuery.HostParse.User, httpLableQuery.HostParse.Repo)
    log.Debugf("httpLable | GitHubReposCacheKey = %s", GitHubReposCacheKey)

    var GitHubRepos *GitHubRepos
    //TODO: проверка времени кэша
    if GitHubReposCacheList[GitHubReposCacheKey] == nil {
        GitHubRepos, err := httpLableQuery.GetGitHubRepos(ctx)
        if err != nil {
            return
        }
        GitHubReposCacheList[GitHubReposCacheKey] = &GitHubReposCache{
            time.Now(),
            GitHubRepos,
        }
    }
    GitHubRepos = GitHubReposCacheList[GitHubReposCacheKey].GitHubRepos

    log.Debugf("httpLable | GitHubRepos = %+v", GitHubRepos)

    ctx.Header("Content-Type", "image/svg+xml")

    var counts []string
    if strings.Index(httpLableQuery.Show, ",watch,") > -1 {
        counts = append(counts, fmt.Sprintf("w: %d", GitHubRepos.WatchersCount))
    }
    if strings.Index(httpLableQuery.Show, ",star,") > -1 {
        counts = append(counts, fmt.Sprintf("s: %d", GitHubRepos.StargazersCount))
    }
    if strings.Index(httpLableQuery.Show, ",fork,") > -1 {
        counts = append(counts, fmt.Sprintf("f: %d", GitHubRepos.ForksCount))
    }
    countString := strings.Join(counts, "; ")

    charWidth := 6.13200

    titleString := `counts`

    widthTitle := float64(len(titleString)) * charWidth
    widthCount := float64(len(countString)) * charWidth
    widthAll := widthTitle + widthCount

    widthTitleString := fmt.Sprintf("%f", widthTitle+6)
    widthAllString := fmt.Sprintf("%f", widthAll+6)

    offsetTitleX := widthTitle/2 + 2
    offsetTitleXString := fmt.Sprintf("%f", offsetTitleX)
    offsetCounX := widthCount/2 + widthTitle + 6
    offsetCounXString := fmt.Sprintf("%f", offsetCounX)

    svgString := `
    <svg xmlns="http://www.w3.org/2000/svg" width="` + widthAllString + `" height="18">
    <linearGradient id="a" x2="0" y2="100%">
        <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
        <stop offset="1" stop-opacity=".1"/>
    </linearGradient>
    <rect rx="3" width="` + widthAllString + `" height="18" fill="#555"/>
    <rect rx="3" x="` + widthTitleString + `" width="` + fmt.Sprintf("%f", widthAll-widthTitle) + `" height="18" fill="#4c1"/>
    <path fill="#4c1" d="M` + widthTitleString + ` 0h4v18h-4z"/>
    <rect rx="3" width="` + widthAllString + `" height="18" fill="url(#a)"/>
    <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
        <text x="` + offsetTitleXString + `" y="14" fill="#010101" fill-opacity=".3">` + titleString + `</text>
        <text x="` + offsetTitleXString + `" y="13">` + titleString + `</text>
        <text x="` + offsetCounXString + `" y="14" fill="#010101" fill-opacity=".3">` + countString + `</text>
        <text x="` + offsetCounXString + `" y="13">` + countString + `</text>
    </g>
    </svg>`

    ctx.String(200, svgString)
}

func (q *httpLableQuery) ParseByCTX(ctx *gin.Context) error {
    if err := ctx.BindQuery(q); err != nil {
        log.Panicf("httpLable | BindQuery ERROR: %s", err)
        return err
    }
    urlParse, err := url.Parse(q.Host)
    if err != nil {
        log.Panicf("httpLable | url.Parse ERROR: %s", err)
        return err
    }
    if urlParse.Host != "github.com" && urlParse.Host != "www.github.com" {
        log.Panic("httpLable | urlParse.Host != github.com")
        return err
    }
    urlParsePath := strings.Split(urlParse.Path, "/")
    q.HostParse.User = urlParsePath[1]
    q.HostParse.Repo = urlParsePath[2]
    q.Show = "," + q.Show + ","
    return nil
}

func (q *httpLableQuery) GetGitHubRepos(ctx *gin.Context) (GitHubRepos *GitHubRepos, err error) {
    res, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/%s", q.HostParse.User, q.HostParse.Repo))
    defer res.Body.Close()
    if err != nil {
        log.Panicf("httpLable | http.Get ERROR: %s", err)
        return nil, err
    }
    robots, err := ioutil.ReadAll(res.Body)
    if err != nil {
        log.Panicf("httpLable | ioutil.ReadAll ERROR: %s", err)
        return nil, err
    }
    err = json.Unmarshal(robots, &GitHubRepos)
    if err != nil {
        log.Panicf("httpLable | json.Unmarshal ERROR: %s", err)
        return nil, err
    }
    return GitHubRepos, nil
}
