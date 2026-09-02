import { Injectable } from '@angular/core';
import { Observable, filter, map } from 'rxjs';

import { ApiEmptyRequest, ApiService } from '@cs/api';

import {
    CreateMcpKeyRequest,
    CreateMcpKeyResponse,
    EmptyMcpKeyResponse,
    GetMcpKeysResponse,
    McpKey
} from '../interfaces';

@Injectable({
    providedIn: 'root'
})
export class McpKeyRequestService {
    constructor(private readonly apiService: ApiService) {}

    getMcpKeys(): Observable<McpKey[]> {
        return this.apiService
            .get<ApiEmptyRequest, GetMcpKeysResponse>('mcp-key/page/1?page_size=100')
            .pipe(map((response) => response.access_tokens));
    }

    /**
     * createMcpKey returns the key with its secret. Unlike a public API token,
     * a key cannot be read back one by one — Public API exposes no such route —
     * so the entity is assembled from the request and the answer to it.
     */
    createMcpKey(request: CreateMcpKeyRequest): Observable<McpKey> {
        return this.apiService.post<CreateMcpKeyRequest, CreateMcpKeyResponse>('mcp-key', request).pipe(
            filter((response) => !!response.id),
            map((response) => ({
                id: response.id,
                name: request.name,
                permissions: request.permissions,
                scopes: request.scopes,
                expires_at: request.expires_at,
                access_token: response.access_token
            }))
        );
    }

    deleteMcpKey(id: string): Observable<string> {
        return this.apiService
            .delete<EmptyMcpKeyResponse>(`mcp-key/${id}`)
            .pipe(map((response) => (response && !Object.keys(response).length ? id : '')));
    }
}
