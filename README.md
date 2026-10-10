<img src="logo.svg" width="96" alt="cs2-demo logo">

# cs2-demo

Pick a downloaded CS2 demo in your browser and play it in the game. No more typing long demo names in the console.

## Install

1. Download `cs2-demo-<version>.exe` from the [latest release](https://github.com/viminizer/cs2-demo/releases/latest).
2. Double-click it. Windows may warn you because the file is not signed: click **More info → Run anyway**.

## Use

1. The left side lists the `.dem`, `.zip` and `.rar` files in your Downloads folder, newest first.
2. Pick a match. The right side lists the maps (`.dem` files) inside it.
3. Pick a map and click **Play**.

If CS2 is closed, the tool starts it and loads the demo.

If CS2 is already open, click **Restart CS2 and play**. CS2 can't take a demo from outside while it runs, so the tool restarts it with the demo.

Demos are extracted to `game\csgo\dl\` with a short name. Delete that folder when it gets big.

## Options

```
cs2-demo-<version>.exe -demos D:\demos      folder with your demos (default: Downloads)
cs2-demo-<version>.exe -steam D:\Steam      Steam folder (default: read from the registry)
cs2-demo-<version>.exe -port 7350           web UI port
```

## Build

```
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o cs2-demo.exe .
```

To release, run `./release.sh v0.0.7 "notes"`. It builds `cs2-demo-v0.0.7.exe` and publishes it.
