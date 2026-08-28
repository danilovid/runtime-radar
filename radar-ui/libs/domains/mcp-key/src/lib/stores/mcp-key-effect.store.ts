import { DateAdapter } from '@koobiq/components/core';
import { DateTime } from 'luxon';
import { HttpErrorResponse } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Action, Store } from '@ngrx/store';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';
import { Observable, of } from 'rxjs';
import { catchError, filter, map, switchMap, take, tap } from 'rxjs/operators';

import { I18nService } from '@cs/i18n';
import { LoadStatus } from '@cs/core';
import { SIGN_OUT_EVENT_ACTION } from '@cs/domains/auth';
import { SWITCH_CLUSTER_EVENT_ACTION } from '@cs/domains/cluster';
import { ApiErrorCode, ApiUtilsService as apiUtils } from '@cs/api';

import { McpKeyRequestService } from '../services/mcp-key-request.service';
import { getMcpKeyLoadStatus } from './mcp-key-selector.store';
import {
    CREATE_MCP_KEY_ENTITY_TODO_ACTION,
    DELETE_ALL_MCP_KEY_ENTITIES_DOC_ACTION,
    DELETE_MCP_KEY_ENTITY_DOC_ACTION,
    DELETE_MCP_KEY_ENTITY_TODO_ACTION,
    LOAD_MCP_KEY_ENTITIES_TODO_ACTION,
    POLLING_LOAD_MCP_KEY_ENTITIES_TODO_ACTION,
    SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION,
    SET_MCP_KEY_ENTITY_DOC_ACTION,
    UPDATE_MCP_KEY_STATE_DOC_ACTION
} from './mcp-key-action.store';
import { McpKey, McpKeyState } from '../interfaces';

@Injectable({
    providedIn: 'root'
})
export class McpKeyEffectStore {
    readonly loadMcpKeys$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(LOAD_MCP_KEY_ENTITIES_TODO_ACTION),
            switchMap(() =>
                this.mcpKeyRequestService.getMcpKeys().pipe(
                    take(1),
                    catchError(() => of(undefined))
                )
            ),
            switchMap((list) => {
                if (list === undefined) {
                    return [
                        UPDATE_MCP_KEY_STATE_DOC_ACTION({
                            loadStatus: LoadStatus.ERROR
                        })
                    ];
                }

                return [
                    SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION({ list }),
                    UPDATE_MCP_KEY_STATE_DOC_ACTION({
                        loadStatus: LoadStatus.LOADED,
                        lastUpdate: this.dateAdapter.today().toMillis()
                    })
                ];
            })
        )
    );

    readonly reloadMcpKeys$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SWITCH_CLUSTER_EVENT_ACTION),
            switchMap(() => this.store.select(getMcpKeyLoadStatus).pipe(take(1))),
            filter((status) => status !== LoadStatus.INIT),
            map(() => LOAD_MCP_KEY_ENTITIES_TODO_ACTION())
        )
    );

    readonly pollingLoadMcpKeys$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(POLLING_LOAD_MCP_KEY_ENTITIES_TODO_ACTION),
            switchMap(() => this.mcpKeyRequestService.getMcpKeys().pipe(take(1))),
            switchMap((list) => [
                SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION({ list }),
                UPDATE_MCP_KEY_STATE_DOC_ACTION({ lastUpdate: this.dateAdapter.today().toMillis() })
            ])
        )
    );

    readonly createMcpKey$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CREATE_MCP_KEY_ENTITY_TODO_ACTION),
            switchMap((action) =>
                this.mcpKeyRequestService.createMcpKey(action.item).pipe(
                    take(1),
                    catchError((error: HttpErrorResponse) => {
                        if (apiUtils.getReasonCode(error) === ApiErrorCode.NAME_MUST_BE_UNIQUE) {
                            this.toastService.show({
                                style: KbqToastStyle.Warning,
                                title: this.i18nService.translate('McpKey.Pseudo.Notification.NameMustBeUnique')
                            });
                        }

                        return of({} as McpKey);
                    })
                )
            ),
            filter((item) => !!item.id),
            map((item) => SET_MCP_KEY_ENTITY_DOC_ACTION({ item })),
            tap(() => {
                this.toastService.show({
                    style: KbqToastStyle.Success,
                    title: this.i18nService.translate('McpKey.Pseudo.Notification.Created')
                });
            })
        )
    );

    readonly deleteMcpKey$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(DELETE_MCP_KEY_ENTITY_TODO_ACTION),
            switchMap((action) =>
                this.mcpKeyRequestService.deleteMcpKey(action.id).pipe(
                    take(1),
                    catchError(() => {
                        this.toastService.show({
                            style: KbqToastStyle.Warning,
                            title: this.i18nService.translate('McpKey.Pseudo.Notification.DeleteFailed')
                        });

                        return of('');
                    })
                )
            ),
            filter((id) => !!id),
            map((id) => DELETE_MCP_KEY_ENTITY_DOC_ACTION({ id })),
            tap(() => {
                this.toastService.show({
                    style: KbqToastStyle.Contrast,
                    title: this.i18nService.translate('McpKey.Pseudo.Notification.Deleted')
                });
            })
        )
    );

    readonly clearState$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SIGN_OUT_EVENT_ACTION),
            switchMap(() => [
                DELETE_ALL_MCP_KEY_ENTITIES_DOC_ACTION(),
                UPDATE_MCP_KEY_STATE_DOC_ACTION({
                    loadStatus: LoadStatus.INIT,
                    lastUpdate: 0
                })
            ])
        )
    );

    constructor(
        private readonly actions$: Actions,
        private readonly dateAdapter: DateAdapter<DateTime>,
        private readonly i18nService: I18nService,
        private readonly mcpKeyRequestService: McpKeyRequestService,
        private readonly store: Store<McpKeyState>,
        private readonly toastService: KbqToastService
    ) {}
}
