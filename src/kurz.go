package main

import (
    "web"
    "strings"
    "godis"
    "fmt"
    "os"
    "url"
    "flag"
    "strconv"
    "time"
    "json"
)

const(
    // special key in redis, that is our global counter
    COUNTER = "__counter__"
    HTTP = "http"
)


var (
    redis *godis.Client
    config *Config
)


type KurzUrl struct{
    Key string
    ShortUrl string
    LongUrl string
    CreationDate int64
    Clicks int64
}

// Converts the KurzUrl to JSON.
func (k KurzUrl) Json()[]byte{
    b, _ := json.Marshal(k)
    return b
}

// Creates a new KurzUrl instance. The Given key, shorturl and longurl will
// be used. Clicks will be set to 0 and CreationDate to time.Nanoseconds()
func NewKurzUrl(key string, shorturl string, longurl string) *KurzUrl{
    kurl := new(KurzUrl)
    kurl.CreationDate = time.Nanoseconds()
    kurl.Key = key
    kurl.LongUrl = longurl
    kurl.ShortUrl = shorturl
    kurl.Clicks = 0
    return kurl
}


// function to resolve a shorturl and redirect
func resolve(ctx *web.Context, short string) {
    redirect, err := redis.Hget(short, "LongUrl")
    if err == nil {
        go redis.Hincrby(short, "Clicks", 1)
        ctx.Redirect(301, redirect.String())
    } else {
        ctx.Redirect(301, "https://www.youtube.com/watch?v=jRHmvy5eaG4")
    }
}


// Determines if the string rawurl is a valid URL to be stored.
func isValidUrl(rawurl string) (u *url.URL, err os.Error){
    if len(rawurl) == 0{
        return nil, os.NewError("empty url")
    }
    // XXX this needs some love...
    if !strings.HasPrefix(rawurl, HTTP){
        rawurl = fmt.Sprintf("%s://%s", HTTP, rawurl)
    }
    return url.Parse(rawurl)
}

// stores a new KurzUrl for the given key, shorturl and longurl. Existing
// ones with the same url will be overwritten
func store(key string, shorturl string, longurl string)*KurzUrl{
    kurl := NewKurzUrl(key, shorturl, longurl)
    go redis.Hset(kurl.Key, "LongUrl", kurl.LongUrl)
    go redis.Hset(kurl.Key, "ShortUrl", kurl.ShortUrl)
    go redis.Hset(kurl.Key, "CreationDate", kurl.CreationDate)
    go redis.Hset(kurl.Key, "Clicks", kurl.Clicks)
    return kurl
}

// loads a KurzUrl instance for the given key. If the key is
// not found, os.Error is returned.
func load(key string) (kurl *KurzUrl, err os.Error){
    if ok, _ := redis.Hexists(key, "ShortUrl"); ok{
        kurl := new(KurzUrl)
        kurl.Key = key
        reply, _ := redis.Hmget(key, "LongUrl", "ShortUrl", "CreationDate", "Clicks")
        kurl.LongUrl, kurl.ShortUrl, kurl.CreationDate, kurl.Clicks =
            reply.Elems[0].Elem.String(), reply.Elems[1].Elem.String(),
            reply.Elems[2].Elem.Int64(), reply.Elems[3].Elem.Int64()
        return kurl, nil
    }
    return nil, os.NewError("unknown key: " + key )
}


// function to shorten and store a url
func shorten(ctx *web.Context, data string){
    host := config.GetStringDefault("hostname", "localhost")
    r, _ := ctx.Request.Params["url"]
    theUrl, err := isValidUrl(string(r))
    if err == nil{
        ctr, _ := redis.Incr(COUNTER)
        encoded := Encode(ctr)
        location := fmt.Sprintf("%s://%s/%s", HTTP, host, encoded)

        kurl := store(encoded, location, theUrl.Raw)

        ctx.SetHeader("Content-Type", "application/json", true)
        ctx.SetHeader("Location", location, true)
        ctx.StartResponse(201)
        ctx.Write(kurl.Json())
        ctx.WriteString("\n")
    }else{
       ctx.Redirect(404, "/")
    }
}


func latest(ctx *web.Context, data string){
    howmany, err := strconv.Atoi64(data)
    if err != nil {
        howmany = 10
    }
    c, _ := redis.Get(COUNTER)
    last := c.Int64()
    upTo := (last - howmany)

    ctx.SetHeader("Content-Type", "application/json", true)
    ctx.WriteString("{ \"urls\" : [")
    for i := last; i > upTo && i > 0; i -= 1{
        kurl, err := load(Encode(i))
        if err == nil{
            ctx.Write(kurl.Json())
            if i != upTo + 1 {
                ctx.WriteString(",")
            }
        }
    }
    ctx.WriteString("] }")
    ctx.WriteString("\n")
}


func bootstrap(path string) os.Error {
    config = NewConfig(path)
    config.Parse()
    host := config.GetStringDefault("redis.address", "tcp:localhost:6379")
    db := config.GetIntDefault("redis.database", 0)
    passwd := config.GetStringDefault("redis.password", "")

    redis = godis.New(host, db, passwd)
    return nil
}


// main function that inits the routes in web.go
func main() {
    flag.Parse()
    cfgFile := flag.Arg(0)
    err := bootstrap(cfgFile)
    if err == nil {
        // this could go to bootstrap as well
        web.Post("/shorten/(.*)", shorten)
        web.Get("/latest/(.*)", latest)
        web.Get("/(.*)", resolve)
        listen := config.GetStringDefault("listen", "0.0.0.0")
        port := config.GetStringDefault("port", "9999")
        web.Run(fmt.Sprintf("%s:%s", listen, port))
    }
}

