export namespace service {
	
	export class AutoImportConfirmRequest {
	    items: typeless.AutoImportCandidate[];
	
	    static createFrom(source: any = {}) {
	        return new AutoImportConfirmRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], typeless.AutoImportCandidate);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AutoImportConfirmResult {
	    accepted_count: number;
	    words: typeless.PendingDictionaryWord[];
	
	    static createFrom(source: any = {}) {
	        return new AutoImportConfirmResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accepted_count = source["accepted_count"];
	        this.words = this.convertValues(source["words"], typeless.PendingDictionaryWord);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AutoImportScanRequest {
	    sources: typeless.AutoImportSource[];
	
	    static createFrom(source: any = {}) {
	        return new AutoImportScanRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sources = this.convertValues(source["sources"], typeless.AutoImportSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Config {
	    userDataPath: string;
	    dbPath: string;
	    apiHost: string;
	    timeoutSec: number;
	    autoImportStatePath: string;
	    cachePath: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userDataPath = source["userDataPath"];
	        this.dbPath = source["dbPath"];
	        this.apiHost = source["apiHost"];
	        this.timeoutSec = source["timeoutSec"];
	        this.autoImportStatePath = source["autoImportStatePath"];
	        this.cachePath = source["cachePath"];
	    }
	}
	export class DictionaryCache {
	    words: typeless.DictionaryWord[];
	    pendingWords: typeless.PendingDictionaryWord[];
	
	    static createFrom(source: any = {}) {
	        return new DictionaryCache(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.words = this.convertValues(source["words"], typeless.DictionaryWord);
	        this.pendingWords = this.convertValues(source["pendingWords"], typeless.PendingDictionaryWord);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
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
	
	export class AutoImportCandidate {
	    term: string;
	    normalized_term: string;
	    platform: string;
	    hits: number;
	    examples: string[];
	
	    static createFrom(source: any = {}) {
	        return new AutoImportCandidate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.term = source["term"];
	        this.normalized_term = source["normalized_term"];
	        this.platform = source["platform"];
	        this.hits = source["hits"];
	        this.examples = source["examples"];
	    }
	}
	export class AutoImportScanResult {
	    scanned_files: number;
	    parsed_messages: number;
	    raw_candidates: number;
	    filtered_candidates: number;
	    items: AutoImportCandidate[];
	
	    static createFrom(source: any = {}) {
	        return new AutoImportScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanned_files = source["scanned_files"];
	        this.parsed_messages = source["parsed_messages"];
	        this.raw_candidates = source["raw_candidates"];
	        this.filtered_candidates = source["filtered_candidates"];
	        this.items = this.convertValues(source["items"], AutoImportCandidate);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AutoImportSource {
	    platform: string;
	    enabled: boolean;
	    workdir: string;
	
	    static createFrom(source: any = {}) {
	        return new AutoImportSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.enabled = source["enabled"];
	        this.workdir = source["workdir"];
	    }
	}
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
	export class PendingDictionaryWord {
	    term: string;
	    platform: string;
	    example: string;
	    status: string;
	    created_at: string;
	    updated_at: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingDictionaryWord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.term = source["term"];
	        this.platform = source["platform"];
	        this.example = source["example"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.error = source["error"];
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

