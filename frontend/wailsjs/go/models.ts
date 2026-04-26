export namespace service {
	
	export class Config {
	    userDataPath: string;
	    dbPath: string;
	    apiHost: string;
	    timeoutSec: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userDataPath = source["userDataPath"];
	        this.dbPath = source["dbPath"];
	        this.apiHost = source["apiHost"];
	        this.timeoutSec = source["timeoutSec"];
	    }
	}
	export class HistoryQuery {
	    limit: number;
	    keyword: string;
	    regex: string;
	    contextMode: string;
	
	    static createFrom(source: any = {}) {
	        return new HistoryQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.limit = source["limit"];
	        this.keyword = source["keyword"];
	        this.regex = source["regex"];
	        this.contextMode = source["contextMode"];
	    }
	}

}

export namespace typeless {
	
	export class DictionaryWord {
	    user_dictionary_id: string;
	    term: string;
	    lang: string;
	    category: string;
	    created_at: string;
	    updated_at: string;
	    auto: boolean;
	    replace: boolean;
	    replace_targets: string[];
	
	    static createFrom(source: any = {}) {
	        return new DictionaryWord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_dictionary_id = source["user_dictionary_id"];
	        this.term = source["term"];
	        this.lang = source["lang"];
	        this.category = source["category"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.auto = source["auto"];
	        this.replace = source["replace"];
	        this.replace_targets = source["replace_targets"];
	    }
	}
	export class ImportResult {
	    TotalInput: number;
	    Unique: number;
	    Skipped: number;
	    Imported: number;
	    Terms: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TotalInput = source["TotalInput"];
	        this.Unique = source["Unique"];
	        this.Skipped = source["Skipped"];
	        this.Imported = source["Imported"];
	        this.Terms = source["Terms"];
	    }
	}
	export class ResetResult {
	    TotalInput: number;
	    Unique: number;
	    Kept: number;
	    Deleted: number;
	    Imported: number;
	
	    static createFrom(source: any = {}) {
	        return new ResetResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TotalInput = source["TotalInput"];
	        this.Unique = source["Unique"];
	        this.Kept = source["Kept"];
	        this.Deleted = source["Deleted"];
	        this.Imported = source["Imported"];
	    }
	}
	export class TranscriptRecord {
	    ID: string;
	    Text: string;
	    CreatedAt: string;
	    AppName: string;
	    BundleID: string;
	    Title: string;
	    WebDomain: string;
	    WebURL: string;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Text = source["Text"];
	        this.CreatedAt = source["CreatedAt"];
	        this.AppName = source["AppName"];
	        this.BundleID = source["BundleID"];
	        this.Title = source["Title"];
	        this.WebDomain = source["WebDomain"];
	        this.WebURL = source["WebURL"];
	    }
	}

}

