'use strict';

import * as App from '../wailsjs/go/main/App.js'

window.onload = async function() {
	try {
		await App.Prepare();
		redrawAll();
	} catch(err) {
		logError(err);
	}
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

window.onclick = async function(ev) {
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
			await App.GoTo("/");
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	let logoutButton = closest(target, "#logoutButton");
	if (logoutButton) {
		try {
			await App.Logout();
			await App.GoTo("/");
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
	let openDirButton = closest(target, "#openDirButton");
	if (openDirButton) {
		try {
			let app = await App.State();
			if (app.Dir != "") {
				await App.OpenDir(app.Dir);
			}
		} catch (err) {
			logError(err);
		}
	}
	let scene = closest(target, "#entryList .scene");
	if (scene) {
		let sceneListExpander = closest(target, ".sceneListExpander");
		if (sceneListExpander) {
			let elem = closest(sceneListExpander, ".element");
			let showing = sceneListExpander.dataset.showing as string;
			if (!showing) {
				showing = "1";
			} else {
				showing = "";
			}
			sceneListExpander.dataset.showing = showing;
			let vers = elem.querySelectorAll(".scene:not(.latest)") as NodeListOf<HTMLElement>;
			for (let v of Array.from(vers)) {
				if (showing) {
					v.classList.remove("hidden");
				} else {
					v.classList.add("hidden");
				}
			}
		} else {
			let scenes = querySelectorAll("#entryList .scene");
			scenes.forEach(s => s.classList.remove("selected"));
			scene.classList.add("selected");
			if (ev.detail == 2) {
				// double click
				let now = Date.now();
				let ellapsed = now - lastSceneClick;
				if (ellapsed < 300) {
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
			toggleNewElementButton(prog).then(redrawAll).catch(logError);
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
		await App.GoTo(path);
		try {
			redrawAll();
		} catch(err) {
			logError(err);
		}
	}
}


let entryList = querySelector("#entryList");

entryList.oncontextmenu = async function(ev) {
	ev.preventDefault();
	let menu = querySelector("#contextMenu");
	menu.style.left = ev.pageX + "px";
	menu.style.top = ev.pageY + "px";
	menu.replaceChildren();
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
	menu.append(label);
	let item = document.createElement("div");
	item.classList.add("contextMenuItem");
	item.innerText = "publish";
	menu.append(item);
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
	// NOTE: metaKey is used instead of both ctrl or alt on mac
	let ctrlLike = ev.ctrlKey || ev.metaKey;
	if (ctrlLike) {
		ev.preventDefault();
		if (ev.key == "r") {
			App.Reload().then(redrawAll).catch(logError);
		}
		if (ev.key == "c") {
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
	}
	let altLike = ev.altKey || ev.metaKey;
	if (altLike) {
		ev.preventDefault();
		if (ev.key == "ArrowLeft") {
			App.GoBack().then(redrawAll).catch(logError);
			return;
		}
		if (ev.key == "ArrowRight") {
			App.GoForward().then(redrawAll).catch(logError);
			return;
		}
	}
	if (ev.key == "F5") {
		ev.preventDefault();
		App.Reload().then(redrawAll).catch(logError);
		return;
	}
	let target = (<HTMLElement> ev.target);
	let newElementFieldInput = closest(target, ".newElementFieldInput");
	if (newElementFieldInput) {
		let input = newElementFieldInput as HTMLInputElement;
		let app = await App.State();
		let oninput = function() {
			if (ev.key != "Enter") {
				return;
			}
			let field = closest(input, ".newElementField");
			let prog = field.dataset.prog as string;
			let name = input.value as string;
			if (name == "") {
				name = "main";
			}
			field.classList.add("hidden");
			App.NewElement(app.Path, name, prog).then(async function() {
				await App.ReloadUserSetting();
				await App.ReloadEntry();
			}).then(redrawAll).catch(logError);
		}
		oninput();
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
	if (app.User == "") {
		currentUser.classList.add("hidden");
		logoutButton.classList.add("hidden");
		loginButton.classList.remove("hidden");
	} else {
		loginButton.classList.add("hidden");
		currentUser.classList.remove("hidden");
		logoutButton.classList.remove("hidden");
		let toks = app.User.split("@")
		if (toks.length == 2 && toks[1] == app.Host) {
			currentUser.innerText = toks[0];
		} else {
			currentUser.innerText = app.User;
		}
	}
}

function redrawOptionBar(app: any) {
	let assignedCheckBox = querySelector("#assignedCheckBox") as HTMLInputElement;
	let reloadAssignedButton = querySelector("#reloadAssignedButton");
	let openDirButton = querySelector("#openDirButton");
	if (app.User == "") {
		assignedCheckBox.disabled = true;
		assignedCheckBox.checked = false;
		reloadAssignedButton.classList.add("disabled");
		openDirButton.dataset.type = "disabled";
		return;
	}
	assignedCheckBox.disabled = false;
	assignedCheckBox.checked = app.Options.AssignedOnly;
	reloadAssignedButton.classList.remove("disabled");
	openDirButton.dataset.type = "";
	if (app.Dir == "") {
		openDirButton.dataset.type = "disabled";
		return;
	}
	if (app.DirExists) {
		openDirButton.dataset.type = "";
	} else {
		openDirButton.dataset.type = "new";
	}
}

function setCurrentPath(app: any) {
	let currentPath = querySelector("#currentPath");
	currentPath.replaceChildren();
	let goto = ""
	let toks = app.Path.split("/").slice(1);
	let root = document.createElement("span");
	root.id = "forgeRoot";
	root.innerText = app.Host;
	root.classList.add("link");
	root.onclick = async function() {
		await App.GoTo("/");
		try {
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	currentPath.append(root);
	for (let t of toks) {
		let gotoPath = goto + "/" + t;
		goto = gotoPath;
		let span = document.createElement("span");
		span.innerText = "/"
		span.classList.add("link");
		currentPath.append(span)
		span = document.createElement("span");
		span.classList.add("link");
		span.innerText = t
		span.onclick = async function() {
			await App.GoTo(gotoPath);
			try {
				redrawAll();
			} catch (err: any) {
				logError(err);
			}
		}
		currentPath.append(span)
	}
	let divider = document.createElement("div");
	divider.classList.add("divider");
	currentPath.append(divider);
	let link = document.createElement("div");
	link.id = "urlLink";
	currentPath.append(link)
}

function redrawEntryList(app: any) {
	let entryList = querySelector("#entryList");
	if (app.User == "") {
		entryList.replaceChildren();
		return;
	}
	if (app.AtLeaf) {
		entryList.replaceChildren();
		for (let e of app.Elements) {
			let elem = document.createElement("div");
			elem.classList.add("element");
			entryList.append(elem);
			let scene = document.createElement("div");
			scene.classList.add("scene");
			scene.classList.add("item");
			scene.classList.add("latest");
			scene.dataset.elem = e.Name;
			scene.dataset.prog = e.Program;
			scene.dataset.ver = "";
			elem.append(scene);
			let expander = document.createElement("div");
			expander.classList.add("sceneListExpander");
			scene.append(expander);
			scene.innerHTML += e.Name + " (" + e.Program + ")";
			let vers = e.Versions.reverse();
			for (let v of vers) {
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
		}
	} else {
		entryList.replaceChildren();
		for (let ent of app.Entries) {
			let div = document.createElement("div");
			div.classList.add("entry");
			div.classList.add("item");
			div.innerText = ent.Name;
			div.onclick = async function() {
				await App.GoTo(ent.Path);
				try {
					redrawAll();
				} catch (err) {
					logError(err);
				}
			}
			entryList.append(div);
		}
	}
}

function redrawNewElementButtons(app: any) {
	let btns = [];
	for (let prog of app.ProgramsInUse) {
		let btn = document.createElement("div");
		btn.classList.add("newElementButton");
		btn.classList.add("button");
		btn.dataset.prog = prog.Name;
		btn.innerText = "+" + prog.Name;
		if (prog.NotFound) {
			btn.classList.add("invalid");
		}
		if (!app.AtLeaf) {
			btn.classList.add("invalid");
		}
		btns.push(btn);
	}
	let elemBtns = querySelector("#newElementButtons");
	elemBtns.replaceChildren(...btns);
}

function redrawRecentPaths(app: any) {
	let cnt = querySelector("#recentPaths");
	cnt.replaceChildren();
	for (let path of app.RecentPaths) {
		let div = document.createElement("div");
		div.classList.add("recentPath");
		div.classList.add("link");
		div.dataset.path = path;
		div.innerText = path;
		cnt.append(div);
	}
}

async function redrawInfoArea(app: any) {
	let area = querySelector("#infoArea");
	if (!app.User) {
		area.replaceChildren();
		return;
	}
	let ents = [...app.ParentEntries];
	ents.push(app.Entry)
	let addEntryInfoDiv = function(ent: any) {
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
		name.classList.add("titleName", "pathLink", "link");
		name.dataset.path = ent.Path;
		name.innerText = ent.Name;
		title.append(name);
		let info = document.createElement("div");
		info.classList.add("titleInfo");
		title.append(info);
		div.append(title)
		return div
	}
	let entDivs = [];
	for (let ent of ents) {
		if (ent.Path == "/") {
			continue;
		}
		if (ent.Type == "show") {
			let entDiv = addEntryInfoDiv(ent);
			let info = entDiv.querySelector(".titleInfo") as HTMLElement;
			info.innerText = ent.Property["sup"].Eval + " / " + ent.Property["pm"].Eval;
			entDivs.push(entDiv);
		} else if (ent.Type == "shot" || ent.Type == "asset") {
			let entDiv = addEntryInfoDiv(ent);
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
			let info = entDiv.querySelector(".titleInfo") as HTMLElement;
			let due = ent.Property["due"].Eval;
			if (due != "") {
				info.innerText = "~ " + due;
			}
			entDivs.push(entDiv);
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
				if (ent.Path == app.Path) {
					entDiv.classList.add("current");
				}
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
				entDivs.push(entDiv);
			}
		}
	}
	area.replaceChildren(...entDivs);
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

function fillAddProgramLinkPopup(app: any) {
	let popup = querySelector("#addProgramLinkPopup");
	popup.replaceChildren();
	let progs = app.Programs.concat(app.LegacyPrograms)
	for (let prog of progs) {
		let div = document.createElement("div");
		div.classList.add("addProgramLinkPopupItem");
		div.classList.add("button");
		if (prog.NotFound) {
			div.classList.add("legacy");
		}
		div.dataset.value = prog.Name;
		div.innerText = prog.Name;
		popup.append(div);
	}
}

async function toggleNewElementButton(prog: string) {
	let app = await App.State();
	try {
		let found = false;
		for (let p of app.ProgramsInUse) {
			if (p.Name == prog) {
				found = true;
				break;
			}
		}
		if (found) {
			await App.RemoveProgramInUse(prog);
		} else {
			await App.AddProgramInUse(prog, app.ProgramsInUse.length);
		}
		await App.ReloadUserSetting();
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
