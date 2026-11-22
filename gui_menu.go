//go:build !darwin && !nogui

package main

import webview "github.com/webview/webview_go"

// menuSysteme : rien à faire hors de macOS (raccourcis de copie gérés par la vue web).
func menuSysteme(webview.WebView) {}
