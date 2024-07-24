'use strict';

import * as App from '../wailsjs/go/main/App.js'

window.onload = async function() {
	try {
		await App.Prepare();
		await redrawAll();
	} catch(err) {
		logError(err);
	}
	let entryList = document.querySelector("#entryList") as HTMLElement;
	let currentEntry = document.querySelector("#currentEntry") as HTMLElement;
	let path = currentEntry.dataset.path as string;
	entryList.dataset.oldPath = path;
}

function closest(from: HTMLElement, query: string): HTMLElement {
	return from.closest(query)!
}

function querySelector(query: string): HTMLElement {
	return document.querySelector(query) as HTMLElement
}

function querySelectorAll(query: string): NodeListOf<HTMLElement> {
	return document.querySelectorAll(query) as NodeListOf<HTMLElement>
}

let lastSceneClick = Date.now();
let HoveringRecentPath: HTMLElement | null = null;

window.onclick = async function(ev) {
	let altLike = ev.altKey || ev.metaKey;
	let target = (<HTMLElement> ev.target);
	let contextMenu = closest(target, "#contextMenu");
	if (!contextMenu) {
		let menu = querySelector("#contextMenu");
		menu.style.display = "none";
	}
	let bookmark = closest(target, ".entryBookmark");
	if (bookmark) {
		try {
			await App.GoTo(bookmark.innerText);
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	let backButton = closest(target, "#backButton");
	if (backButton) {
		App.GoBack().then(redrawAll).catch(logError);
	}
	let forwardButton = closest(target, "#forwardButton");
	if (forwardButton) {
		App.GoForward().then(redrawAll).catch(logError);
	}
	let reloadButton = closest(target, "#reloadButton");
	if (reloadButton) {
		App.ReloadEntry().then(redrawAll).catch(logError);
	}
	let loginButton = closest(target, "#loginButton");
	if (loginButton) {
		try {
			await App.Login();
			let app = await App.State();
			let path = "/";
			if (app.RecentPaths.length != 0) {
				path = app.RecentPaths[0]
			}
			await App.GoTo(path);
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	let logoutButton = closest(target, "#logoutButton");
	if (logoutButton) {
		try {
			await App.Logout();
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	let urlLink = closest(target, "#urlLink");
	if (urlLink) {
		let app = await App.State();
		App.OpenURL(app.Path).catch(logError);
	}
	let reloadAssignedButton = closest(target, "#reloadAssignedButton");
	if (reloadAssignedButton) {
		if (!reloadAssignedButton.classList.contains("disabled")) {
			App.ReloadAssigned().then(redrawAll).catch(logError);
		}
	}
	let recentsButton = closest(target, "#recentsButton");
	if (recentsButton) {
		let recentPaths = querySelector("#recentPaths");
		if (recentPaths.classList.contains("hidden")) {
			recentsButton.classList.add("on");
			recentPaths.classList.remove("hidden");
		} else {
			recentsButton.classList.remove("on");
			recentPaths.classList.add("hidden");
		}
	}
	let openDirButton = closest(target, ".openDirButton");
	if (openDirButton) {
		let path = openDirButton.dataset.path as string;
		if (!path) {
			logError(openDirButton.dataset.err);
			return;
		}
		try {
			await App.OpenDir(path);
		} catch (err) {
			logError(err);
		}
	}
	let scene = closest(target, "#entryList .scene");
	if (scene) {
		let expander = closest(target, ".sceneListExpander");
		if (expander) {
			let showing = expander.dataset.showing as string;
			let expand = !showing
			toggleSceneListExpander(expander, expand);
		} else {
			let scenes = querySelectorAll("#entryList .scene");
			scenes.forEach(s => s.classList.remove("selected"));
			scene.classList.add("selected");
			if (ev.detail == 2) {
				// double click
				let now = Date.now();
				let ellapsed = now - lastSceneClick;
				if (ellapsed < 300) {
					openScene(scene);
				}
			} else {
				lastSceneClick = Date.now()
			}
		}
	}
	let recentPath = closest(target, ".recentPath");
	if (recentPath) {
		try {
			let path = recentPath.dataset.path as string;
			await App.GoTo(path);
			redrawAll();
		} catch (err) {
			logError(err);
		}
	}
	let addProgramLink = closest(target, "#addProgramLink");
	if (addProgramLink) {
		toggleAddProgramLinkPopup();
	}
	let addProgramLinkPopup = closest(target, "#addProgramLinkPopup");
	if (addProgramLinkPopup) {
		let item = closest(target, ".addProgramLinkPopupItem");
		if (item) {
			toggleAddProgramLinkPopup();
			let prog = item.dataset.value as string;
			toggleNewElementButton(prog).catch(logError);
		}
	}
	if (!addProgramLink && !addProgramLinkPopup) {
		hideAddProgramLinkPopup();
	}
	let newElementButton = closest(target, ".newElementButton");
	if (newElementButton) {
		if (!newElementButton.classList.contains("invalid")) {
			let prog = newElementButton.dataset.prog as string;
			addNewElementField(prog);
		}
	} else {
		let newElementField = closest(target, ".newElementField");
		if (newElementField == null) {
			removeNewElementField();
		}
	}
	let pathLink = closest(target, ".pathLink");
	if (pathLink) {
		let path = pathLink.dataset.path as string;
		App.GoTo(path).then(redrawAll).catch(logError);
		return;
	}
	let entryLink = closest(target, ".entryLink");
	if (entryLink) {
		let path = entryLink.innerText as string;
		App.GoTo(path).then(redrawAll).catch(logError);
		return;
	}
	let pathText = closest(target, ".pathText");
	if (pathText) {
		let path = pathText.innerText;
		if (altLike) {
			try {
				await App.Open(path)
			} catch (err) {
				log("failed to open: " + path);
			}
		} else {
			copyToClipboard(path);
			log("path copied: " + path);
		}
	}
}

window.onmouseover = async function(ev) {
	let target = (<HTMLElement> ev.target);
	let altLike = ev.altKey || ev.metaKey;
	let recentPath = closest(target, ".recentPath");
	if (recentPath) {
		HoveringRecentPath = recentPath;
		if (altLike) {
			showThumbnailPopup(recentPath);
		}
	} else {
		HoveringRecentPath = null;
		hideThumbnailPopup();
	}
}

async function openScene(scene: HTMLElement) {
	try {
		let app = await App.State();
		let elem = scene.dataset.elem as string;
		let ver = scene.dataset.ver as string;
		let prog = scene.dataset.prog as string;
		await App.OpenScene(app.Path, elem, ver, prog);
		await App.ReloadUserSetting();
		redrawAll();
	} catch(err) {
		logError(err);
	}
}

function toggleSceneListExpander(expander: HTMLElement, expand: boolean) {
	if (expand) {
		expander.dataset.showing = "1";
	} else {
		expander.dataset.showing = "";
	}
	let elem = expander.closest(".element") as HTMLElement;
	let vers = elem.querySelectorAll(".scene:not(.latest)") as NodeListOf<HTMLElement>;
	for (let v of Array.from(vers)) {
		if (expand) {
			v.classList.remove("hidden");
		} else {
			v.classList.add("hidden");
		}
	}
}

function showThumbnailPopup(recentPath: HTMLElement) {
	let popup = querySelector("#thumbnailPopup");
	popup.classList.remove("on");
	let item = document.createElement("img") as HTMLImageElement;
	item.style.width = "96px";
	item.style.height = "54px";
	let path = recentPath.dataset.path as string;
	App.GetThumbnail(path).then(function(thumb) {
		item.src = "data:image/png;base64," + thumb.Data;
		popup.classList.add("on");
	}).catch(logError);
	let rect = recentPath.getBoundingClientRect();
	popup.style.top = rect.bottom + 6 + "px";
	popup.style.left = rect.left + "px";
	popup.replaceChildren(item);
}

function hideThumbnailPopup() {
	let popup = querySelector("#thumbnailPopup");
	popup.classList.remove("on");
}

let entryList = querySelector("#entryList");

entryList.oncontextmenu = async function(ev) {
	ev.preventDefault();
	let menu = querySelector("#contextMenu");
	menu.style.left = ev.pageX + "px";
	menu.style.top = ev.pageY + "px";
	let target = ev.target as HTMLElement;
	let entItem = target.closest(".scene.item") as HTMLElement;
	if (!entItem) {
	    menu.style.display = "none";
		return;
	}
	menu.style.display = "flex";
	let app = await App.State();
	let elem = entItem.dataset.elem as string;
	let prog = entItem.dataset.prog as string;
	let ver = entItem.dataset.ver as string;
	if (ver == "") {
		ver = await App.LastVersionOfElement(app.Path, elem, prog)
	}
	let label = document.createElement("div");
	label.classList.add("contextMenuLabel");
	label.innerText = elem + " / " + ver;
	let item = document.createElement("div");
	item.classList.add("contextMenuItem");
	item.innerText = "publish";
	menu.replaceChildren(label, item);
}

let contextMenu = querySelector("#contextMenu");

contextMenu.oncontextmenu = function(ev) {
	ev.preventDefault();
	let menu = querySelector("#contextMenu");
	menu.style.display = "none";
}

window.onchange = async function(ev) {
	let target = (<HTMLElement> ev.target);

	let availableProgramSelect = closest(target, "#availableProgramSelect");
	if (availableProgramSelect) {
		let select = availableProgramSelect as HTMLInputElement;
		if (select.value != "") {
			console.log(select.value);
		}
	}
	let assignedCheckBox = closest(target, "#assignedCheckBox");
	if (assignedCheckBox) {
		try {
			let checkbox = assignedCheckBox as HTMLInputElement;
			await App.SetAssignedOnly(checkbox.checked);
			await App.ReloadEntry();
		} catch (err) {
			// The option may not be restored whenuser run the App next time.
			// But not a fatal error. Just log it.
			logError(err);
		}
		try {
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
}

window.onkeydown = async function(ev) {
	let app = await App.State();

	// NOTE: metaKey is used instead of both ctrl or alt on mac
	let ctrlLike = ev.ctrlKey || ev.metaKey;
	if (ctrlLike) {
		ev.preventDefault();
		if (ev.code == "KeyQ") {
			App.Quit();
		}
		if (ev.code == "KeyR") {
			App.ReloadEntry().then(redrawAll).catch(logError);
		}
		if (ev.code == "KeyC") {
			let sel = document.querySelector<HTMLElement>(".item.selected");
			if (!sel) {
				return;
			}
			let app = await App.State();
			let elem = sel.dataset.elem as string;
			let ver = sel.dataset.ver as string;
			let prog = sel.dataset.prog as string;
			let scene = await App.SceneFile(app.Path, elem, ver, prog);
			copyToClipboard(scene);
			log("path copied: " + scene);
		}
		if (ev.code == "KeyV") {
			let path = await App.GetClipboardText();
			// use the first line, when pasted text is multi-line.
			path = path.trim();
			path = path.split("\n")[0];
			path = path.trim();
			let app = await App.State();
			let host = "https://" + app.Host
			if (path.startsWith(host)) {
				path = path.slice(host.length);
			}
			if (path.startsWith(app.Host)) {
				path = path.slice(app.Host.length);
			}
			if (!path.startsWith("/")) {
				logError("invalid path: " + path);
				return;
			}
			App.GoTo(path).then(async function() {
				await redrawAll();
				let ent = document.querySelector("#currentEntry") as HTMLElement;
				let entryList = document.querySelector("#entryList") as HTMLElement;
				entryList.dataset.oldPath = ent.dataset.path;
			}).catch(logError);
			return;
		}
	}
	let altLike = ev.altKey || ev.metaKey;
	if (altLike) {
		ev.preventDefault();
		if (ev.code == "ArrowLeft") {
			App.GoBack().then(redrawAll).catch(logError);
			return;
		}
		if (ev.code == "ArrowRight") {
			App.GoForward().then(redrawAll).catch(logError);
			return;
		}
		if (HoveringRecentPath) {
			showThumbnailPopup(HoveringRecentPath);
		}
	}
	if (ev.code == "F5") {
		ev.preventDefault();
		App.ReloadEntry().then(redrawAll).catch(logError);
		return;
	}

	let target = (<HTMLElement> ev.target);
	let newElementFieldInput = closest(target, ".newElementFieldInput");
	if (newElementFieldInput) {
		let input = newElementFieldInput as HTMLInputElement;
		let app = await App.State();
		let oninput = function() {
			if (ev.code != "Enter") {
				return;
			}
			let field = closest(input, ".newElementField");
			let prog = field.dataset.prog as string;
			let name = input.value as string;
			field.classList.add("hidden");
			App.NewElement(app.Path, name, prog).then(async function() {
				await App.ReloadUserSetting();
				await App.ReloadEntry();
			}).then(redrawAll).catch(logError);
		}
		oninput();
		return;
	}

	// keyboard navigation
	if (ev.code == "ArrowUp") {
		let entryList = document.querySelector("#entryList") as HTMLElement;
		entryList.dataset.oldPath = ""
		let items = document.querySelectorAll(".item:not(.hidden)") as NodeListOf<HTMLElement>;
		if (!items) {
			// there isn't any item.
			return;
		}
		let idx = -1
		for (let i = 0; i < items.length; i++) {
			let it = items[i];
			if (it.classList.contains("selected")) {
				idx = i;
				break;
			}
		}
		if (idx != -1) {
			let sel = items[idx];
			sel.classList.remove("selected");
		}
		if (idx <= 0) {
			idx = items.length;
		}
		idx -= 1;
		let sel = items[idx];
		sel.classList.add("selected");
		sel.scrollIntoView({block: "nearest"});
		if (!app.AtLeaf) {
			let itemPath = sel.dataset.path as string;
			entryList.dataset.oldPath = itemPath;
		}
		return;
	} else if (ev.code == "ArrowDown") {
		let entryList = document.querySelector("#entryList") as HTMLElement;
		entryList.dataset.oldPath = ""
		let items = document.querySelectorAll(".item:not(.hidden)") as NodeListOf<HTMLElement>;
		if (items.length == 0) {
			// there isn't any item.
			return;
		}
		let idx = -1
		for (let i = 0; i < items.length; i++) {
			let it = items[i];
			if (it.classList.contains("selected")) {
				idx = i;
				break;
			}
		}
		if (idx != -1) {
			let sel = items[idx];
			sel.classList.remove("selected");
		}
		idx += 1;
		if (idx == items.length) {
			idx = 0;
		}
		let sel = items[idx];
		sel.classList.add("selected");
		sel.scrollIntoView({block: "nearest"});
		if (!app.AtLeaf) {
			let itemPath = sel.dataset.path as string;
			entryList.dataset.oldPath = itemPath;
		}
		return;
	} else if (ev.code == "ArrowLeft") {
		let entryList = document.querySelector("#entryList") as HTMLElement;
		entryList.dataset.search  = "";
		let sel = document.querySelector(".item:not(.hidden).selected") as HTMLElement;
		if (sel) {
			if (sel.classList.contains("scene")) {
				let ver = sel.dataset.ver as string;
				if (ver == "") {
					let expander = sel.querySelector(".sceneListExpander") as HTMLElement;
					let showing = expander.dataset.showing as string ;
					if (showing != "") {
						toggleSceneListExpander(expander, false);
						return;
					}
				} else {
					sel.classList.remove("selected");
					let elem = sel.dataset.elem as string;
					let prog = sel.dataset.prog as string;
					sel = document.querySelector(`.scene[data-elem='${elem}'][data-prog='${prog}'][data-ver='']`) as HTMLElement;
					let expander = sel.querySelector(".sceneListExpander") as HTMLElement;
					toggleSceneListExpander(expander, false);
					sel.classList.add("selected");
					return;
				}
			}
		}
		let ent = document.querySelector("#currentEntry") as HTMLElement;
		let path = ent.dataset.path as string;
		if (path == "/") {
			return;
		}
		let toks = path.split("/");
		toks.pop();
		let parent = toks.join("/");
		if (parent == "") {
			parent = "/";
		}
		App.GoTo(parent).then(async function() {
			await redrawAll();
			setSelected(true);
		}).catch(logError);
		return;
	} else if (ev.code == "ArrowRight") {
		let entryList = document.querySelector("#entryList") as HTMLElement;
		entryList.dataset.search  = "";
		let sel = document.querySelector(".item:not(.hidden).selected") as HTMLElement;
		if (sel) {
			if (sel.classList.contains("scene")) {
				let ver = sel.dataset.ver as string;
				if (ver == "") {
					let expander = sel.querySelector(".sceneListExpander") as HTMLElement;
					let showing = expander.dataset.showing as string;
					if (showing == "") {
						toggleSceneListExpander(expander, true);
						return;
					}
				}
				return;
			}
			await onclickElement(sel);
			setSelected(true);
		}
		return;
	} else if (ev.code == "Enter" || ev.code == "NumpadEnter") {
		let entryList = document.querySelector("#entryList") as HTMLElement;
		entryList.dataset.search  = "";
		let sel = document.querySelector(".item:not(.hidden).selected") as HTMLElement;
		if (sel) {
			if (sel.classList.contains("scene")) {
				openScene(sel);
				return;
			}
			await onclickElement(sel);
			setSelected(true);
		}
		return;
	}

	if (!app.AtLeaf) {
		// there are already hidden scenes, will conflict with search
		if (
			ev.code.startsWith("Key") || ev.code.startsWith("Digit") || ev.code.startsWith("Numpad") ||
			ev.code == "Minus" || ev.code == "Backspace" || ev.code == "Escape"
		) {
			// wierd, but Minus also represent underscore
			let entryList = document.querySelector("#entryList") as HTMLElement;
			let search = entryList.dataset.search as string;
			if (!search) {
				search = "";
			}
			if (ev.code == "Escape") {
				search = "";
			} else if (ev.code == "Backspace") {
				if (search != "") {
					search = search.slice(0, search.length-1);
				}
			} else {
				search += ev.key;
			}
			entryList.dataset.search = search;
			let items = entryList.querySelectorAll(".item") as NodeListOf<HTMLElement>;
			for (let it of Array.from(items)) {
				if (!it.innerText.includes(search)) {
					it.classList.add("hidden");
				} else {
					it.classList.remove("hidden");
				}
			}
			let sel = document.querySelector(".item.selected") as HTMLElement;
			if (!sel || (sel && sel.classList.contains("hidden"))) {
				// need to select a new item
				if (sel) {
					// hidden item shouldn't be selected.
					sel.classList.remove("selected");
				}
				let firstItem = document.querySelector(".item:not(.hidden)") as HTMLElement;
				if (firstItem) {
					firstItem.classList.add("selected");
				}
			}
			sel = document.querySelector(".item.selected") as HTMLElement;
			if (sel) {
				sel.scrollIntoView();
			}
			if (search != "") {
				log("searching: " + search);
			} else {
				log("clear search");
			}
		}
	}
}

window.onkeyup = async function(ev) {
	let altLike = ev.altKey || ev.metaKey;
	if (!altLike) {
		hideThumbnailPopup();
	}
}

function copyToClipboard(msg: string) {
	let t = document.createElement("textarea");
	t.value = msg;
	t.style.position = "fixed"; // prevents scrolling
	document.body.appendChild(t);
	t.focus();
	t.select();
	try {
		document.execCommand("copy");
	} catch (err) {
		logError(err);
		return;
	}
	document.body.removeChild(t);
}

function clearLog() {
	let bar = querySelector("#statusBar");
	bar.innerText = "";
}

function log(msg: string) {
	let m = msg.split("\n")[0];
	let bar = querySelector("#statusBar");
	bar.innerText = m;
}

function logError(err: any) {
	console.log(err);
	let e = err.toString().split("\n")[0];
	let bar = querySelector("#statusBar");
	bar.innerText = e;
}

async function redrawAll(): Promise<void> {
	let app = await App.State();
	console.log(app);
	try {
		clearLog();
		redrawLoginArea(app);
		redrawOptionBar(app);
		setCurrentPath(app);
		redrawCurrentEntry(app);
		redrawEntryList(app);
		redrawInfoArea(app).catch(logError);
		redrawProgramsBar(app);
		redrawRecentPaths(app);
	} catch (err) {
		logError(err);
	}
}

function redrawProgramsBar(app: any) {
	fillAddProgramLinkPopup(app);
	redrawNewElementButtons(app);
}

function redrawLoginArea(app: any) {
	let loginButton = querySelector("#loginButton");
	let logoutButton = querySelector("#logoutButton");
	let currentUser = querySelector("#currentUser");
	if (!app.User) {
		currentUser.classList.add("hidden");
		logoutButton.classList.add("hidden");
		loginButton.classList.remove("hidden");
	} else {
		loginButton.classList.add("hidden");
		currentUser.classList.remove("hidden");
		logoutButton.classList.remove("hidden");
		if (app.User.Called) {
			currentUser.innerText = app.User.Called;
		} else {
			currentUser.innerText = app.User.Name;
		}
	}
}

async function redrawOptionBar(app: any) {
	let assignedCheckBox = querySelector("#assignedCheckBox") as HTMLInputElement;
	let reloadAssignedButton = querySelector("#reloadAssignedButton");
	let openDirButton = querySelector("#openCurrentDir");
	if (app.User == "") {
		assignedCheckBox.disabled = true;
		assignedCheckBox.checked = false;
		reloadAssignedButton.classList.add("disabled");
		openDirButton.dataset.path = "";
		return;
	}
	assignedCheckBox.disabled = false;
	assignedCheckBox.checked = app.Options.AssignedOnly;
	reloadAssignedButton.classList.remove("disabled");
	await refreshOpenDirButton(openDirButton, app.Path);
}

function setCurrentPath(app: any) {
	let currentPath = querySelector("#currentPath");
	let children = [];
	let goto = ""
	let toks = app.Path.split("/").slice(1);
	let root = document.createElement("span");
	root.id = "forgeRoot";
	root.innerText = app.Host;
	root.classList.add("link");
	root.onclick = async function() {
		await App.GoTo("/");
		try {
			await redrawAll();
		} catch (err: any) {
			logError(err);
		}
		setSelected(false);
	}
	children.push(root);
	for (let t of toks) {
		let gotoPath = goto + "/" + t;
		goto = gotoPath;
		let span = document.createElement("span");
		span.innerText = "/"
		span.classList.add("link");
		children.push(span)
		span = document.createElement("span");
		span.classList.add("link");
		span.innerText = t
		span.onclick = async function() {
			await App.GoTo(gotoPath);
			try {
				await redrawAll();
			} catch (err: any) {
				logError(err);
			}
			setSelected(false);
		}
		children.push(span)
	}
	let divider = document.createElement("div");
	divider.classList.add("divider");
	children.push(divider);
	let link = document.createElement("div");
	link.id = "urlLink";
	children.push(link)
	currentPath.replaceChildren(...children);
}

function redrawCurrentEntry(app: any) {
	let ent = querySelector("#currentEntry") as HTMLElement;
	ent.dataset.path = app.Path;
	let entryThumbnail = querySelector("#currentEntryThumbnail") as HTMLImageElement;
	App.GetThumbnail(app.Path).then(function(thumb) {
		entryThumbnail.src = "data:image/png;base64," + thumb.Data;
	}).catch(logError);
}

function redrawEntryList(app: any) {
	let entryList = querySelector("#entryList");
	entryList.dataset.search = "";
	let children = [];
	if (app.User == "") {
		entryList.replaceChildren();
		return;
	}
	if (app.AtLeaf) {
		for (let e of app.Elements) {
			let elem = document.createElement("div");
			elem.classList.add("element");
			let scene = document.createElement("div");
			scene.classList.add("scene");
			scene.classList.add("item");
			scene.classList.add("latest");
			scene.dataset.elem = e.Name;
			scene.dataset.prog = e.Program;
			scene.dataset.ver = "";
			elem.append(scene);
			let thumbEl = document.createElement("img") as HTMLImageElement;
			thumbEl.classList.add("thumbnail");
			scene.append(thumbEl);
			App.GetThumbnail(app.Path).then(function(thumb) {
				let thumbEl = scene.querySelector(".thumbnail") as HTMLImageElement;
				thumbEl.src = "data:image/png;base64," + thumb.Data;
			}).catch(logError);
			let expander = document.createElement("div");
			expander.classList.add("sceneListExpander");
			expander.dataset.showing = "";
			scene.append(expander);
			if (e.Name == "") {
				scene.innerHTML += "[main] (" + e.Program + ")";
			} else {
				scene.innerHTML += e.Name + " (" + e.Program + ")";
			}
			for (let v of e.Versions) {
				let scene = document.createElement("div");
				scene.classList.add("scene");
				scene.classList.add("item");
				scene.classList.add("hidden");
				scene.dataset.elem = e.Name;
				scene.dataset.prog = e.Program;
				scene.dataset.ver = v.Name;
				scene.innerText = v.Name;
				elem.append(scene);
			}
			children.push(elem);
		}
	} else {
		for (let ent of app.Entries) {
			let div = document.createElement("div") as HTMLElement;
			div.classList.add("entry");
			div.classList.add("item");
			let thumbEl = document.createElement("img") as HTMLImageElement;
			thumbEl.classList.add("thumbnail");
			div.append(thumbEl);
			App.GetThumbnail(ent.Path).then(function(thumb) {
				let thumbEl = div.querySelector(".thumbnail") as HTMLImageElement;
				thumbEl.src = "data:image/png;base64," + thumb.Data;
			}).catch(logError);
			div.innerHTML += ent.Name;
			div.dataset.path = ent.Path;
			div.onclick = async function() {
				await onclickElement(div);
				setSelected(false);
			}
			children.push(div);
		}
	}
	entryList.replaceChildren(...children);
}

function setSelected(fallbackSelection: boolean) {
	let entryList = document.querySelector("#entryList") as HTMLElement;
	let firstItem = entryList.querySelector(".item:not(.hidden)") as HTMLElement;
	let currentEntry = document.querySelector("#currentEntry") as HTMLElement;
	let path = currentEntry.dataset.path as string;
	let oldPath = entryList.dataset.oldPath as string;
	if (!oldPath) {
		oldPath = "";
	}
	let sel = null;
	if (oldPath.length > path.length && oldPath.slice(0, path.length) == path) {
		if (path == "/") {
			path = "";
		}
		let rest = oldPath.slice(path.length+1);
		let selName = rest.split("/")[0];
		if (selName) {
			// don't need to reset old path
			let selPath = path + "/" + selName;
			sel = document.querySelector(".item[data-path='" + selPath + "']");
		} else {
			// the entry might be deleted or so...
			entryList.dataset.oldPath = path;
		}
	} else {
		entryList.dataset.oldPath = path;
		if (fallbackSelection) {
			sel = firstItem;
		}
	}
	if (sel) {
		sel.classList.add("selected");
		sel.scrollIntoView();
		// show 1 + 1/2 items above selected item, if possible.
		entryList.scrollTop -= 70;
		sel.scrollIntoView({block: "nearest"});
	}
}

async function onclickElement(div: HTMLElement) {
	await App.GoTo(div.dataset.path as string);
	try {
		await redrawAll();
	} catch (err) {
		logError(err);
	}
}

async function redrawNewElementButtons(app: any) {
	let elemBtns = querySelector("#newElementButtons");
	let children = [];
	for (let prog of app.ProgramsInUse) {
		let btn = document.createElement("div");
		btn.classList.add("newElementButton");
		btn.classList.add("button");
		let p = await App.Program(prog);
		btn.dataset.prog = prog;
		btn.innerText = "+" + prog;
		if (!p) {
			btn.classList.add("invalid");
		}
		if (!app.AtLeaf) {
			btn.classList.add("invalid");
		}
		children.push(btn);
	}
	elemBtns.replaceChildren(...children);
}

function redrawRecentPaths(app: any) {
	let cnt = querySelector("#recentPaths");
	let children = [];
	for (let path of app.RecentPaths) {
		let div = document.createElement("div");
		div.classList.add("recentPath");
		div.classList.add("link");
		div.dataset.path = path;
		div.innerText = path;
		children.push(div);
	}
	cnt.replaceChildren(...children);
}

function isRecent(then: any) {
	let now = Date.now();
	let delta = now - then;
	let day = 24 * 60 * 60 * 1000;
	return delta < day;
}

function addEntryInfoDiv(ent: any) {
	let div = document.createElement("div");
	div.classList.add("entryInfo");
	div.dataset.path = ent.Path;
	div.dataset.type = ent.Type;
	let title = document.createElement("div");
	title.classList.add("title");
	let dot = document.createElement("div");
	dot.classList.add("statusDot", "hidden");
	title.append(dot);
	let name = document.createElement("div");
	name.classList.add("titleName");
	name.innerText = ent.Name;
	let updated = new Date(ent.UpdatedAt);
	if (isRecent(updated)) {
		let dot = document.createElement("div");
		dot.classList.add("recentlyUpdatedDot");
		dot.classList.add("big");
		name.append(dot);
	}
	title.append(name);
	let divider = document.createElement("div");
	divider.classList.add("divider");
	title.append(divider);
	let info = document.createElement("div");
	info.classList.add("titleInfo");
	title.append(info);
	div.append(title)
	return div
}

async function redrawInfoArea(app: any) {
	let area = querySelector("#infoArea");
	if (!app.User) {
		area.replaceChildren();
		return;
	}
	let ents = [...app.ParentEntries];
	ents.push(app.Entry)

	let children = [];
	for (let ent of ents) {
		// draw entry info from root to current entry
		if (ent.Path == "/") {
			continue;
		}
		let entProps = [] as any[];
		for (let prop in ent.Property) {
			if (!prop.startsWith(".")) {
				entProps.push(prop);
			}
		}
		if (entProps.length == 0) {
			continue;
		}
		entProps = await App.SortEntryProperties(entProps, ent.Type);
		let entDiv = addEntryInfoDiv(ent);
		let titleDiv = entDiv.querySelector(".title") as HTMLElement;
		let titleNameDiv = entDiv.querySelector(".titleName") as HTMLElement;
		let plistTglDiv = document.createElement("div");
		plistTglDiv.classList.add("propertyListToggle");
		let imageDiv = document.createElement("div");
		imageDiv.classList.add("image");
		plistTglDiv.append(imageDiv);
		titleNameDiv.onclick = function() {
			let on = plistTglDiv.classList.contains("on");
			on = !on;
			let propsDiv = entDiv.querySelector(".entryProperties") as HTMLElement;
			if (on) {
				plistTglDiv.classList.add("on");
				propsDiv.classList.remove("hidden");
			} else {
				plistTglDiv.classList.remove("on");
				propsDiv.classList.add("hidden");
			}
		}
		let openDirButton = document.createElement("div");
		openDirButton.classList.add("openDirButton")
		refreshOpenDirButton(openDirButton, ent.Path)
		titleNameDiv.append(plistTglDiv);
		titleDiv.append(openDirButton);

		let statusProp = ent.Property["status"];
		if (statusProp) {
			let status = statusProp.Eval;
			let color = await App.StatusColor(ent.Type, status);
			if (!color) {
				color = "#dddddd"
			}
			let dot = entDiv.querySelector(".statusDot") as HTMLElement;
			dot.title = status;
			if (!status) {
				dot.title = "(none)";
			}
			dot.style.backgroundColor = color;
			dot.style.border = "1px solid " + color + "bb";
			dot.classList.remove("hidden");
		}
		let propsDiv = document.createElement("div");
		propsDiv.classList.add("entryProperties");
		propsDiv.classList.add("hidden");
		entDiv.append(propsDiv);
		let exposedDiv = document.createElement("div");
		exposedDiv.classList.add("exposedProperties");
		entDiv.append(exposedDiv);
		let redrawExposedProperties = async function(ent: any) {
			let app = await App.State();
			let exposedProps = app.ExposedProperties[ent.Type];
			let exposed = new Set(exposedProps);
			let tglDivs = entDiv.querySelectorAll(".propertyToggle");
			for (let tglDiv of Array.from(tglDivs)) {
				tglDiv.classList.remove("on");
			}
			for (let prop of exposed.keys()) {
				let tglDiv = entDiv.querySelector(`.propertyToggle[data-entpath="${ent.Path}"][data-name="${prop}"]`);
				if (!tglDiv) {
					continue;
				}
				tglDiv.classList.add("on");
			}
			let children = [];
			for (let prop of entProps) {
				if (!exposed.has(prop)) {
					continue
				}
				let p = ent.Property[prop];
				let propDiv = document.createElement("div");
				propDiv.classList.add("property");
				let nameDiv = document.createElement("div");
				nameDiv.classList.add("propertyName");
				nameDiv.innerText = p.Name;
				let updated = new Date(p.UpdatedAt);
				if (isRecent(updated)) {
					let dot = document.createElement("div");
					dot.classList.add("recentlyUpdatedDot");
					nameDiv.append(dot);
				}
				let valueDiv = document.createElement("div");
				valueDiv.classList.add("propertyValue");
				let lines = p.Eval.split(/\r?\n/);
				for (let l of lines) {
					if (l.trim() == "") {
						let br = document.createElement("br");
						valueDiv.append(br);
						continue;
					}
					let d = document.createElement("div");
					d.innerText = l;
					if (p.Type == "entry_link") {
						d.classList.add("entryLink");
					} else {
						if (l.startsWith("/")) {
							d.classList.add("pathText");
						}
					}
					valueDiv.append(d);
				}
				propDiv.append(nameDiv, valueDiv);
				children.push(propDiv);
			}
			exposedDiv.replaceChildren(...children);
		}
		for (let prop of entProps) {
			let p = ent.Property[prop];
			let cellDiv = document.createElement("div");
			cellDiv.classList.add("propertyToggleCell");
			let tglDiv = document.createElement("div");
			tglDiv.classList.add("propertyToggle");
			tglDiv.dataset.entpath = ent.Path;
			tglDiv.dataset.name = p.Name;
			tglDiv.innerText = p.Name;
			let updated = new Date(p.UpdatedAt);
			if (isRecent(updated)) {
				let dot = document.createElement("div");
				dot.classList.add("recentlyUpdatedDot");
				tglDiv.append(dot);
			}
			tglDiv.onclick = function() {
				App.ToggleExposeProperty(ent.Type, p.Name).then(function() {
					redrawExposedProperties(ent);
				}).catch(logError);
			}
			cellDiv.append(tglDiv);
			propsDiv.append(cellDiv);
		}
		await redrawExposedProperties(ent);
		children.push(entDiv);
	}
	for (let ent of ents) {
		// when we are in a shot or asset, it will show the parts
		// so we can navigate conviniently
		if (!(ent.Type == "shot" || ent.Type == "asset")) {
			continue
		}
		let parts = [];
		try {
			parts = await App.ListAllEntries(ent.Path);
		} catch(err) {
			logError(err);
			return;
		}
		for (let ent of parts) {
			let entDiv = addEntryInfoDiv(ent);
			entDiv.classList.add("sub");
			let titleDiv = entDiv.querySelector(".title") as HTMLElement;
			titleDiv.classList.add("pathLink", "link");
			titleDiv.dataset.path = ent.Path;
			let statusProp = ent.Property["status"];
			if (statusProp) {
				let status = statusProp.Eval;
				let color = await App.StatusColor(ent.Type, status);
				if (!color) {
					color = "#dddddd"
				}
				let dot = entDiv.querySelector(".statusDot") as HTMLElement;
				dot.title = status;
				if (!status) {
					dot.title = "(none)";
				}
				dot.style.backgroundColor = color+"dd";
				dot.style.border = "1px solid " + color;
				dot.classList.remove("hidden");
			}
			let info = entDiv.querySelector(".titleInfo") as HTMLElement;
			info.innerText = ent.Property["assignee"].Eval;
			children.push(entDiv);
		}
	}
	area.replaceChildren(...children);
}

async function refreshOpenDirButton(btn: any, ent: any) {
	let path = "";
	try {
		path = await App.Dir(ent);
	} catch (err: any) {
		btn.dataset.path = "";
		btn.dataset.err = err;
		return;
	}
	btn.dataset.path = path;
	if (path == "") {
		return;
	}
	btn.innerHTML = "";
	try {
		let exists = await App.DirExists(path);
		if (!exists) {
			let newDir = document.createElement("div");
			newDir.classList.add("newDir")
			newDir.innerText = "+"
			btn.append(newDir)
		}
	} catch (err: any) {
		btn.dataset.path = "";
		btn.dataset.err = err;
	}
}

function toggleAddProgramLinkPopup() {
	let popup = querySelector("#addProgramLinkPopup");
	let hidden = popup.classList.contains("hidden");
	// should unhide the popup to get bounding rect
	popup.classList.remove("hidden");
	let rect = popup.getBoundingClientRect();
	if (hidden) {
		popup.style.top = String(-rect.height-4) + "px";
		popup.classList.remove("hidden");
	} else {
		popup.classList.add("hidden");
	}
}

function hideAddProgramLinkPopup() {
	let popup = querySelector("#addProgramLinkPopup");
	popup.classList.add("hidden");
}

async function fillAddProgramLinkPopup(app: any) {
	let popup = querySelector("#addProgramLinkPopup");
	let children = [];
	let progs = app.Programs.concat(app.LegacyPrograms)
	for (let prog of progs) {
		let div = document.createElement("div");
		div.classList.add("addProgramLinkPopupItem");
		div.classList.add("button");
		let p = await App.Program(prog);
		if (!p) {
			div.classList.add("legacy");
		}
		div.dataset.value = prog;
		div.innerText = prog;
		children.push(div);
	}
	popup.replaceChildren(...children);
}

async function toggleNewElementButton(prog: string) {
	let app = await App.State();
	try {
		let found = false;
		for (let p of app.ProgramsInUse) {
			if (p == prog) {
				found = true;
				break;
			}
		}
		if (found) {
			await App.RemoveProgramInUse(prog);
		} else {
			await App.AddProgramInUse(prog, app.ProgramsInUse.length);
		}
		app = await App.State();
		await redrawNewElementButtons(app);
	} catch (err) {
		logError(err);
	}
}

function addNewElementField(prog: string) {
	removeNewElementField();
	let field = document.createElement("div");
	field.classList.add("newElementField");
	field.dataset.prog = prog;
	let list = querySelector("#entryList");
	list.append(field);
	let input = document.createElement("input");
	input.classList.add("newElementFieldInput");
	field.append(input);
	let span = document.createElement("span");
	span.innerText = " (" + prog + ")";
	field.append(span);
	input.focus();
}

function removeNewElementField() {
	let field = querySelector(".newElementField");
	if (field) {
		field.remove();
	}
}
