# go read

a google reader clone built with go on app engine and angularjs

## how to host your own

1. checkout the code
1. create a new app engine application
1. install the app engine SDK
1. set your GOPATH (to something like `/home/user/mygo`
1. run `go get -d github.com/mjibson/goread`
1. in `app.yaml`, change the first line to contain the name of the application you just created
1. in the `goapp` folder, copy `settings.go.dist` to `settings.go`

[optional steps if you want google reader import support]

1. sign up for some API keys at the [google apis console](https://code.google.com/apis/console/)
1. fill in values for localhost and whatever your hostname is in the `init()` function of `settings.go`
1. get a google analytics key and put it into the appropriate field in `settings.go`

[and finally, deploy]

1. deploy with `appcfg.py`
