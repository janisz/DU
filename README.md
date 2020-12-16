# DziennikUstaw

Dziennik Ustaw Twitter Bot![CI](https://github.com/janisz/DU/workflows/CI/badge.svg)<a href="https://www.statuscake.com" title="Website Uptime Monitoring"><img src="https://app.statuscake.com/button/index.php?Track=QDJOsFpvjh&Days=1000&Design=3" /></a>

---

## What it does?

1. Post all new Acts from [Dziennik Ustaw](dziennikustaw.gov.pl/) to [Twitter](https://twitter.com/Dziennik_Ustaw)
2. Like Tweets that mention Dziennik Ustaw
3. Reply to Tweets that mention particular act

## Why?

To get more visibility over the latest legislation changes in Poland

## How to Run?

1. Create developer profile for [Twitter API](https://developer.twitter.com) with [Read & Write permissions](https://developer.twitter.com/en/docs/apps/app-permissions)
2. Run (set `DRY=1` for DRY RUN – not posting anything to GitHub)

```
consumerKey=? consumerSecret=? accessToken=? accessSecret=? go run main.go
```
