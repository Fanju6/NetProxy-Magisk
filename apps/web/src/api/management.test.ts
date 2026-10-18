import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { ApiClient } from './client'
import { managementRequest, readConfig } from './management'
import { MockTransport } from '@/mocks/transport'
import { configSchema, snapshotSchema, type ApiRequest } from './contracts'

describe('restricted management requests', () => {
  it.each([
    { method: 'GET', path: '/config', query: { target: 'api.json' } },
    { method: 'GET', path: '/config', query: { target: 'module', private: 'true' } },
    {
      method: 'PUT',
      path: '/config',
      query: { target: 'runtime/inbound.json' },
      body: { revision: 'r', content: '{}' },
    },
    {
      method: 'PUT',
      path: '/config',
      query: { target: 'inbound' },
      body: { revision: 'r', content: '{}', path: '/data/adb' },
    },
    { method: 'GET', path: '/config', query: { target: 'singbox/rules/local/../../api.json' } },
    { method: 'GET', path: '/logs', query: { lines: '2001' } },
    { method: 'GET', path: '/diagnostics/export', query: { destination: '/tmp' } },
    {
      method: 'POST',
      path: '/nodes',
      body: { group_id: 'default', content: '{}', allow_insecure: false, path: '/tmp' },
    },
  ] satisfies ApiRequest[])('rejects arbitrary parameters: %j', (request) =>
    expect(managementRequest(request)).toBeUndefined(),
  )

  it('preserves revision and content in a fixed operation, not a native URL', () => {
    expect(
      managementRequest({
        method: 'PUT',
        path: '/config',
        query: { target: 'inbound/app' },
        body: { revision: 'read-revision', content: '{"app":{}}' },
      }),
    ).toEqual({
      operation: 'config_apply',
      input: { target: 'inbound/app', revision: 'read-revision', content: '{"app":{}}' },
    })
  })
  it('keeps partitions independent and rejects stale revisions', async () => {
    const api = new ApiClient(new MockTransport(0))
    const auto = await readConfig(api, 'module/auto_start')
    const wifi = await readConfig(api, 'module/wifi')
    await api.request(
      {
        method: 'PUT',
        path: '/config',
        query: { target: wifi.target },
        body: {
          revision: wifi.revision,
          content:
            '{"wifi":{"enabled":true,"mode":"blacklist","blacklist":["Comma, SSID"],"whitelist":[],"proxy_on_non_wifi":true}}',
        },
      },
      z.unknown(),
    )
    await api.request(
      {
        method: 'PUT',
        path: '/config',
        query: { target: auto.target },
        body: { revision: auto.revision, content: '{"auto_start":true}' },
      },
      z.unknown(),
    )
    await expect(
      api.request(
        {
          method: 'PUT',
          path: '/config',
          query: { target: auto.target },
          body: { revision: auto.revision, content: '{"auto_start":false}' },
        },
        z.unknown(),
      ),
    ).rejects.toMatchObject({ code: 'config.conflict', status: 409 })
    expect((await readConfig(api, 'module/wifi')).content).toContain('Comma, SSID')
    expect(JSON.parse((await readConfig(api, 'module')).content)).toMatchObject({
      auto_start: true,
    })
  })
  it('validates without committing and never changes the revision on failure', async () => {
    const api = new ApiClient(new MockTransport(0))
    const before = await readConfig(api, 'module/auto_start')
    const request = {
      method: 'POST',
      path: '/config/validate',
      query: { target: before.target },
      body: { revision: before.revision, content: '{"auto_start":true}' },
    } satisfies ApiRequest
    await api.request(request, z.unknown())
    expect(
      await api.request(
        { method: 'GET', path: '/config', query: { target: before.target } },
        configSchema,
      ),
    ).toEqual(before)
    await expect(
      api.request({ ...request, body: { ...request.body, content: '{invalid' } }, z.unknown()),
    ).rejects.toMatchObject({ status: 422 })
  })
  it('adds, edits, reads and removes local nodes without altering URL groups', async () => {
    const api = new ApiClient(new MockTransport(0))
    const content = JSON.stringify({
      outbounds: [
        { type: 'socks', tag: 'Anonymous fixture', server: '127.0.0.1', server_port: 1080 },
      ],
    })
    await api.request(
      {
        method: 'POST',
        path: '/nodes',
        body: { group_id: 'default', content, allow_insecure: false },
      },
      z.unknown(),
    )
    expect(
      (
        await api.request(
          {
            method: 'GET',
            path: '/nodes/get',
            query: { group_id: 'default', node_tag: 'Anonymous fixture' },
          },
          z.object({ outbounds: z.array(z.object({ tag: z.string() })) }),
        )
      ).outbounds[0]?.tag,
    ).toBe('Anonymous fixture')
    await api.request(
      {
        method: 'PUT',
        path: '/nodes',
        body: {
          group_id: 'default',
          node_tag: 'Anonymous fixture',
          content: content.replace('Anonymous fixture', 'Edited fixture'),
          allow_insecure: false,
        },
      },
      z.unknown(),
    )
    await api.request(
      {
        method: 'DELETE',
        path: '/nodes',
        query: { group_id: 'default', node_tag: 'Edited fixture' },
      },
      z.unknown(),
    )
    const snapshot = await api.request({ method: 'GET', path: '/nodes/snapshot' }, snapshotSchema)
    expect(
      snapshot.groups
        .find((item) => item.group.id === 'default')
        ?.nodes.some((item) => item.tag.includes('fixture')),
    ).toBe(false)
  })
})
