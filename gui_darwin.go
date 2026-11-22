//go:build darwin && !nogui

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

// cofMenuMac pose la barre de menus de l'application. Sans menu Édition, macOS ne relie pas
// ⌘C, ⌘V, ⌘X, ⌘A et ⌘Z aux champs de la page ; ⌘Q et ⌘W ferment la fenêtre (performClose:),
// ce qui termine COF DM proprement (l'univers est sauvegardé et fermé), là où terminate:
// quitterait sans attendre.
static void cofMenuMac(void) {
	@autoreleasepool {
		NSMenu *barre = [[NSMenu alloc] init];

		NSMenuItem *itemApp = [[NSMenuItem alloc] init];
		NSMenu *app = [[NSMenu alloc] initWithTitle:@"COF DM"];
		[app addItemWithTitle:@"Masquer COF DM" action:@selector(hide:) keyEquivalent:@"h"];
		NSMenuItem *autres = [app addItemWithTitle:@"Masquer les autres" action:@selector(hideOtherApplications:) keyEquivalent:@"h"];
		[autres setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagOption];
		[app addItemWithTitle:@"Tout afficher" action:@selector(unhideAllApplications:) keyEquivalent:@""];
		[app addItem:[NSMenuItem separatorItem]];
		[app addItemWithTitle:@"Quitter COF DM" action:@selector(performClose:) keyEquivalent:@"q"];
		[itemApp setSubmenu:app];
		[barre addItem:itemApp];

		NSMenuItem *itemEdition = [[NSMenuItem alloc] init];
		NSMenu *edition = [[NSMenu alloc] initWithTitle:@"Édition"];
		[edition addItemWithTitle:@"Annuler" action:@selector(undo:) keyEquivalent:@"z"];
		NSMenuItem *retablir = [edition addItemWithTitle:@"Rétablir" action:@selector(redo:) keyEquivalent:@"z"];
		[retablir setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagShift];
		[edition addItem:[NSMenuItem separatorItem]];
		[edition addItemWithTitle:@"Couper" action:@selector(cut:) keyEquivalent:@"x"];
		[edition addItemWithTitle:@"Copier" action:@selector(copy:) keyEquivalent:@"c"];
		[edition addItemWithTitle:@"Coller" action:@selector(paste:) keyEquivalent:@"v"];
		[edition addItemWithTitle:@"Tout sélectionner" action:@selector(selectAll:) keyEquivalent:@"a"];
		[itemEdition setSubmenu:edition];
		[barre addItem:itemEdition];

		NSMenuItem *itemFenetre = [[NSMenuItem alloc] init];
		NSMenu *fenetre = [[NSMenu alloc] initWithTitle:@"Fenêtre"];
		[fenetre addItemWithTitle:@"Réduire" action:@selector(performMiniaturize:) keyEquivalent:@"m"];
		[fenetre addItemWithTitle:@"Réduire/agrandir" action:@selector(performZoom:) keyEquivalent:@""];
		NSMenuItem *plein = [fenetre addItemWithTitle:@"Plein écran" action:@selector(toggleFullScreen:) keyEquivalent:@"f"];
		[plein setKeyEquivalentModifierMask:NSEventModifierFlagCommand | NSEventModifierFlagControl];
		[fenetre addItem:[NSMenuItem separatorItem]];
		[fenetre addItemWithTitle:@"Fermer" action:@selector(performClose:) keyEquivalent:@"w"];
		[itemFenetre setSubmenu:fenetre];
		[barre addItem:itemFenetre];

		[NSApp setMainMenu:barre];
		[NSApp setWindowsMenu:fenetre];
	}
}
*/
import "C"

import webview "github.com/webview/webview_go"

// menuSysteme pose la barre de menus de macOS, une fois l'application lancée (sur le thread
// principal, par la boucle de la fenêtre).
func menuSysteme(w webview.WebView) {
	w.Dispatch(func() { C.cofMenuMac() })
}
