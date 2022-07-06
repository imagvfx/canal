// Cynhyrchwyd y ffeil hon yn awtomatig. PEIDIWCH Â MODIWL
// This file is automatically generated. DO NOT EDIT
import {main} from '../models';

export function GoBack():Promise<string>;

export function GoForward():Promise<string>;

export function Logout():void;

export function SetAssignedOnly(arg1:boolean):void;

export function WaitLogin(arg1:string):Promise<Error>;

export function AddProgramInUse(arg1:string,arg2:number):Promise<Error>;

export function ClearEntries():void;

export function CurrentPath():Promise<string>;

export function EntryEnvirons(arg1:string):Promise<Array<main.Environ>|Error>;

export function OpenLoginPage(arg1:string):Promise<Error>;

export function SessionUser():Promise<string>;

export function GoTo(arg1:string):void;

export function IsLeaf(arg1:string):Promise<boolean>;

export function ListEntries():Promise<Array<string>|Error>;

export function Login():Promise<string|Error>;

export function NewElement(arg1:string,arg2:string,arg3:string):Promise<Error>;

export function Programs():Promise<Array<string>>;

export function ProgramsInUse():Promise<Array<string>|Error>;

export function RemoveProgramInUse(arg1:string):Promise<Error>;
