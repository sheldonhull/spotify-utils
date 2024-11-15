# Spotify Utils

[![forthebadge](https://forthebadge.com/images/featured/featured-gluten-free.svg)](https://forthebadge.com)
[![forthebadge](https://forthebadge.com/images/badges/0-percent-optimized.svg)](https://forthebadge.com)
[![forthebadge](https://forthebadge.com/images/badges/code-written-by-chatgpt-ai-ftw.svg)](https://forthebadge.com)

Bulk unfavorite all the albums in your Spotify library.

## Requirements

Setup at [developer dashboard](https://developer.spotify.com/dashboard).
Add callback: `http://127.0.0.1:4001/callback`.

## Usage

- load `.env` with direnv or desired tool.
- `go run main.go`

## Why

I wanted to reset my library and while I found some actions to easily clear playlists, I didn't find anything for removing all the albums.

I also found this interesting as it works with Oauth waiting for callback.

If anything is ugly in this, I blame GPT. 😂
