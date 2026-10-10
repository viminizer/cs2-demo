<img src="logo.svg" width="96" alt="cs2-demo logo">

# cs2-demo

Pick a downloaded CS2 demo in your browser and play it in the game. No more typing long demo names in the console.

## Install

1. Download `cs2-demo.exe` from the [latest release](https://github.com/viminizer/cs2-demo/releases/latest).
2. Double-click `cs2-demo.exe`. Windows may warn you because the file is not signed: click **More info → Run anyway**.

## Use

1. The left side lists the `.dem`, `.zip` and `.rar` files in your Downloads folder, newest first.
2. Pick a match. The right side lists the maps (`.dem` files) inside it.
3. Pick a map and click **Play**.

If CS2 is closed, the tool starts it and loads the demo. Later demos load into that same running game.

If you started CS2 from Steam yourself, the tool can't reach it. Click **Restart CS2 and play**, or add `-netconport 2121` to CS2's launch options in Steam so it always works.

Demos are extracted to `game\csgo\dl\` with a short name. Delete that folder when it gets big.

## Options

```
cs2-demo.exe -demos D:\demos      folder with your demos (default: Downloads)
cs2-demo.exe -steam D:\Steam      Steam folder (default: read from the registry)
cs2-demo.exe -port 7350           web UI port
```

## Build

```
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o cs2-demo.exe .
```
