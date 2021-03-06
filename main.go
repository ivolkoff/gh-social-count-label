package main

import (
    "fmt"
    "strings"
    "net/url"
    "net/http"
    "io/ioutil"
    "encoding/json"
    "time"
    "os"
    "github.com/gin-contrib/cache"
    "github.com/gin-contrib/cache/persistence"
    log "github.com/sirupsen/logrus"
    "github.com/gin-gonic/gin"
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

var err error
var GITHUB_CLIENT_ID string
var GITHUB_CLIENT_SECRET string

func main() {
    PORT := os.Getenv("PORT")
    if PORT == "" {
        log.Fatal("$PORT must be set")
    }

    GITHUB_CLIENT_ID = os.Getenv("GITHUB_CLIENT_ID")
    if GITHUB_CLIENT_ID == "" {
        log.Fatal("$GITHUB_CLIENT_ID must be set")
    }

    GITHUB_CLIENT_SECRET = os.Getenv("GITHUB_CLIENT_SECRET")
    if GITHUB_CLIENT_SECRET == "" {
        log.Fatal("GITHUB_CLIENT_SECRET must be set")
    }

    if os.Getenv("APP_ENV") == "local" {
        log.SetLevel(log.DebugLevel)
    } else {
        log.SetLevel(log.InfoLevel)
    }

    var REDIS_URL *url.URL
    REDIS_URL, err = url.Parse(os.Getenv("REDIS_URL"))
    if err != nil {
        log.Fatalf("RedisURL parse ERROR: ", err)
    }
    REDIS_URL_USER_PASSWORD, _ := REDIS_URL.User.Password()
    REDIS_STORE := persistence.NewRedisCache(REDIS_URL.Host, REDIS_URL_USER_PASSWORD, time.Second)
    if err != nil {
        log.Fatalf("Redis init | ERROR: ", err)
    }

    router := gin.Default()

    router.GET("/ping", func(c *gin.Context) {
        c.JSON(200, ApiResponse{
            Code:    200,
            Message: "pong",
        })
    })

    router.GET("/lable", cache.CachePage(REDIS_STORE, time.Minute, httpLable))

    router.NoRoute(func(c *gin.Context) {
        c.JSON(404, ApiResponse{
            Code:    404,
            Message: "Not found",
        })
    })

    if err := router.Run(":" + PORT); err != nil {
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
    log.Infof("httpLable | httpLableQuery.HostParse = %+v", httpLableQuery.HostParse)

    GitHubRepos, err := httpLableQuery.GetGitHubRepos(ctx)
    if err != nil {
        return
    }

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

    charWidth := 6.33200

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
    res, err := http.Get(fmt.Sprintf("https://api.github.com/repos/%s/%s?client_id=%s&client_secret=%s", q.HostParse.User, q.HostParse.Repo, GITHUB_CLIENT_ID, GITHUB_CLIENT_SECRET))
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
    //log.Panicf("httpLable | robots: %s", robots)
    err = json.Unmarshal(robots, &GitHubRepos)
    if err != nil {
        log.Panicf("httpLable | json.Unmarshal ERROR: %s", err)
        return nil, err
    }
    return GitHubRepos, nil
}
