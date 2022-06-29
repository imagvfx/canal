'use strict';

import * as App from '../wailsjs/go/main/App.js'

window.onload = function() {
	redrawAll();
}

function closest(from: HTMLElement, query: string): HTMLElement {
	return from.closest(query)!
}

function querySelector(query: string): HTMLElement {
	return document.querySelector(query) as HTMLElement
}

window.onclick = async function(ev) {
	let target = (<HTMLElement> ev.target);

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
		redrawAll();
	}
	let loginButton = closest(target, "#loginButton");
	if (loginButton) {
		await App.Login();
		await resetProgramsInUse();
		await App.GoTo("/");
		try {
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
	let logoutButton = closest(target, "#logoutButton");
	if (logoutButton) {
		await App.Logout();
		await resetProgramsInUse();
		await App.GoTo("/");
		try {
			redrawAll();
		} catch (err: any) {
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
			let v = item.dataset.value as string;
			toggleNewElementButton(v);
			toggleAddProgramLinkPopup();
		}
	}
	let newElementButton = closest(target, ".newElementButton");
	if (newElementButton) {
		let prog = newElementButton.dataset.program as string;
		showNewElementField(prog);
	}
	let newElementField = closest(target, "#newElementField");
	if (newElementButton == null && newElementField == null) {
		hideNewElementField();
	}
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
		let checkBox = assignedCheckBox as HTMLInputElement;
		await App.SetAssignedOnly(checkBox.checked);
		try {
			redrawAll();
		} catch (err: any) {
			logError(err);
		}
	}
}

window.onkeydown = async function(ev) {
	let target = (<HTMLElement> ev.target);

	let newElementFieldInput = closest(target, "#newElementFieldInput");
	if (newElementFieldInput) {
		let input = newElementFieldInput as HTMLInputElement;
		let oninput = function() {
			if (ev.key != "Enter") {
				return;
			}
			let field = closest(input, "#newElementField");
			let prog = field.dataset.program as string;
			App.CurrentPath().then(function(path: string) {
				let name = input.value as string;
				if (name == "") {
					name = "main";
				}
				App.NewElement(path, name, prog).catch(logError);
			});
		}
		oninput();
	}
}

function clearLog() {
	let bar = querySelector("#statusBar");
	bar.innerText = "";
}

function logError(err: any) {
	console.log("here")
	console.log(err);
	let e = err.split("\n").slice(0);
	let bar = querySelector("#statusBar");
	bar.innerText = e;
}

async function redrawAll(): Promise<void> {
	clearLog();
	redrawLoginArea();
	redrawOptionBar();
	App.CurrentPath().then(function(path) {
		setCurrentPath(path);
		checkLeaf(path);
		redrawEntryList();
	}).catch(logError);
	fillAddProgramLinkPopup();
	redrawNewElementButtons();
	hideNewElementField();
}

function redrawLoginArea() {
	App.SessionUser().then(function(user) {
		let loginButton = querySelector("#loginButton");
		let logoutButton = querySelector("#logoutButton");
		let currentUser = querySelector("#currentUser");
		if (user == "") {
			currentUser.classList.add("hidden");
			logoutButton.classList.add("hidden");
			loginButton.classList.remove("hidden");
		} else {
			loginButton.classList.add("hidden");
			currentUser.classList.remove("hidden");
			logoutButton.classList.remove("hidden");
			currentUser.innerText = user;
		}
	}).catch(logError);
}

function redrawOptionBar() {
	App.SessionUser().then(function(user) {
		let assignedCheckBox = querySelector("#assignedCheckBox") as HTMLInputElement;
		if (user == "") {
			assignedCheckBox.disabled = true;
		} else {
			assignedCheckBox.disabled = false;
		}
	})
}

function setCurrentPath(path: string) {
	let currentPath = querySelector("#currentPath");
	currentPath.replaceChildren();
	let goto = ""
	let toks = path.split("/").slice(1);
	let root = document.createElement("span");
	root.id = "forgeRoot";
	root.innerText = "forge:";
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
		currentPath.append(span)
		span = document.createElement("span");
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
}

function checkLeaf(path: string) {
	App.IsLeaf(path).then(function(leaf) {
		let right = querySelector("#right");
		if (leaf) {
			right.classList.add("leaf");
		} else {
			right.classList.remove("leaf");
		}
	});
}

function redrawEntryList() {
	let list = querySelector("#entryList");
	// cannot use removeChildren,
	// #entryList has newElementField as well.
	let oldEnts = list.querySelectorAll(".entry") as NodeListOf<HTMLElement>;
	for (let ent of Array.from(oldEnts)) {
		list.removeChild(ent);
	}

	App.ListEntries().then(async function(arg: string[] | Error) {
		let ents = arg as string[];
		for (let ent of ents) {
			let toks = ent.split("/");
			let name = toks[toks.length-1];
			let div = document.createElement("div");
			div.classList.add("entry");
			div.innerText = name;
			div.onclick = async function() {
				await App.GoTo(ent);
				try {
					redrawAll();
				} catch (err) {
					logError(err);
				}
			}
			list.append(div);
		}
	}).catch(logError);
}

async function redrawNewElementButtons() {
	let progs: string[] = [];
	try {
		progs = await App.ProgramsInUse() as string[];
	} catch (err) {
		logError(err);
	}
	let btns = querySelector("#newElementButtons");
	btns.replaceChildren();
	for (let prog of progs) {
		let btn = document.createElement("div");
		btn.classList.add("newElementButton");
		btn.dataset.program = prog;
		btn.innerText = "+" + prog;
		btns.append(btn);
	}
}

function toggleAddProgramLinkPopup() {
	let popup = querySelector("#addProgramLinkPopup");
	let rect = popup.getBoundingClientRect();
	if (popup.classList.contains("hidden")) {
		popup.style.top = String(-rect.height) + "px";
		popup.classList.remove("hidden");
	} else {
		popup.classList.add("hidden");
	}
}

function fillAddProgramLinkPopup() {
	let popup = querySelector("#addProgramLinkPopup");
	popup.replaceChildren();
	App.Programs().then(function(progs) {
		for (let prog of progs) {
			let div = document.createElement("div");
			div.classList.add("addProgramLinkPopupItem");
			div.dataset.value = prog;
			div.innerText = prog;
			popup.append(div);
		}
	}).catch(logError);
}

async function resetProgramsInUse() {
	let progs: string[] = [];
	try {
		progs = await App.ProgramsInUse() as string[];
	} catch (err: any) {
		logError(err);
	}
	let btns = querySelector("#newElementButtons");
	btns.replaceChildren();
	for (let prog of progs) {
		let btn = document.createElement("div");
		btn.classList.add("newElementButton");
		btn.dataset.program = prog;
		btn.innerText = "+" + prog;
		btns.append(btn);
	}
}

async function toggleNewElementButton(prog: string) {
	let btns = querySelector("#newElementButtons");
	let btn = btns.querySelector(`.newElementButton[data-program=${prog}]`);
	if (btn) {
		try {
			await App.RemoveProgramInUse(prog);
		} catch (err) {
			logError(err);
		}
		btn.remove();
	} else {
		App.AddProgramInUse(prog, btns.children.length).then(function() {
			let btn = document.createElement("div");
			btn.classList.add("newElementButton");
			btn.dataset.program = prog;
			btn.innerText = "+" + prog;
			btns.append(btn);
		}).catch(logError);
	}
}

function showNewElementField(prog: string) {
	let field = querySelector("#newElementField");
	field.dataset.program = prog;
	let span = field.querySelector("#newElementFieldProgram") as HTMLElement;
	span.innerText = "- " + prog;
	field.classList.remove("hidden");
	let input = field.querySelector("#newElementFieldInput") as HTMLElement;
	input.focus();
}

function hideNewElementField() {
	let field = querySelector("#newElementField");
	field.dataset.program = "";
	field.classList.add("hidden");
}
