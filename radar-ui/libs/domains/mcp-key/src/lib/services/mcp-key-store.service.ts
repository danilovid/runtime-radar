import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { Store } from '@ngrx/store';

import { LoadStatus } from '@cs/core';

import { CREATE_MCP_KEY_ENTITY_TODO_ACTION, DELETE_MCP_KEY_ENTITY_TODO_ACTION } from '../stores/mcp-key-action.store';
import { CreateMcpKeyRequest, McpKey, McpKeyState } from '../interfaces';
import { getMcpKeyLoadStatus, getMcpKeys } from '../stores/mcp-key-selector.store';

@Injectable({
    providedIn: 'root'
})
export class McpKeyStoreService {
    readonly mcpKeys$: Observable<McpKey[]> = this.store.select(getMcpKeys);

    readonly loadStatus$: Observable<LoadStatus> = this.store.select(getMcpKeyLoadStatus);

    constructor(private readonly store: Store<McpKeyState>) {}

    createMcpKey(item: CreateMcpKeyRequest) {
        this.store.dispatch(CREATE_MCP_KEY_ENTITY_TODO_ACTION({ item }));
    }

    deleteMcpKey(id: string) {
        this.store.dispatch(DELETE_MCP_KEY_ENTITY_TODO_ACTION({ id }));
    }
}
