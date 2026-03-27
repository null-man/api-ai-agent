"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/components/auth-provider";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  getProvisionSettings,
  listProvisionProviders,
  saveProvisionProvider,
  testProvisionProvider,
  updateProvisionSettings,
  type ProvisionProviderEntry,
  type ProvisionSettingsData,
} from "@/lib/api";

const emptyForm: ProvisionSettingsData["provision"] = {
  model: {
    provider: "minimax",
    id: "MiniMax-M2.5-highspeed",
    base_url: "https://api.minimax.io/v1",
    api: "openai-completions",
    auth: "api-key",
    api_key: "",
  },
  telegram: {
    dm_policy: "open",
    group_policy: "allowlist",
  },
  providers: [],
};

const emptyProviderForm = {
  name: "",
  id: "",
  base_url: "",
  api: "openai-completions",
  auth: "api-key",
  api_key: "",
  set_default: false,
};

export default function SettingsPage() {
  const { isAuthed } = useAuth();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testingName, setTestingName] = useState("");
  const [form, setForm] = useState<ProvisionSettingsData["provision"]>(emptyForm);
  const [providerForm, setProviderForm] = useState(emptyProviderForm);
  const [providers, setProviders] = useState<ProvisionProviderEntry[]>([]);

  const sortedProviders = useMemo(
    () => [...providers].sort((a, b) => a.name.localeCompare(b.name)),
    [providers]
  );

  const fetchSettings = useCallback(async () => {
    try {
      setLoading(true);
      const [settingsRes, providersRes] = await Promise.all([
        getProvisionSettings(),
        listProvisionProviders(),
      ]);
      setForm(settingsRes.data.provision || emptyForm);
      setProviders(providersRes.data.providers || []);
    } catch (err) {
      toast.error("Failed to load settings: " + (err as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isAuthed) {
      fetchSettings();
    }
  }, [isAuthed, fetchSettings]);

  const handleSave = async () => {
    try {
      setSaving(true);
      await updateProvisionSettings({ provision: form });
      toast.success("默认提供商配置已保存，ClawHost 正在重启");
      await fetchSettings();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleSaveProvider = async () => {
    try {
      setSaving(true);
      await saveProvisionProvider(providerForm);
      toast.success(providerForm.set_default ? "提供商已保存并设为默认" : "提供商已保存");
      setProviderForm(emptyProviderForm);
      await fetchSettings();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleUseAsDefault = async (provider: ProvisionProviderEntry) => {
    try {
      setSaving(true);
      await updateProvisionSettings({
        provision: {
          ...form,
          model: {
            provider: provider.name,
            id: provider.id,
            base_url: provider.base_url,
            api: provider.api,
            auth: provider.auth,
            api_key: provider.api_key,
          },
        },
      });
      toast.success("已切换默认提供商，ClawHost 正在重启");
      await fetchSettings();
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleTestProvider = async (provider: ProvisionProviderEntry) => {
    try {
      setTestingName(provider.name);
      const res = await testProvisionProvider(provider.name);
      if (res.data.success) {
        toast.success(`${provider.name} 测试通过`);
      } else {
        toast.error(`${provider.name} 测试失败：HTTP ${res.data.status_code}`);
      }
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setTestingName("");
    }
  };

  if (!isAuthed) return null;

  return (
    <div className="p-4 md:p-6 space-y-4 md:space-y-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-xl md:text-2xl font-bold">Settings</h1>
        <p className="text-sm text-muted-foreground">
          管理全局默认提供商、已配置提供商列表，以及 Telegram 默认渠道策略。保存后会自动重启 ClawHost 主服务。
        </p>
      </div>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">默认提供商配置</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="provider">Provider</Label>
            <Input
              id="provider"
              value={form.model.provider}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, provider: e.target.value } }))}
              placeholder="minimax"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="model-id">Model ID</Label>
            <Input
              id="model-id"
              value={form.model.id}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, id: e.target.value } }))}
              placeholder="MiniMax-M2.5-highspeed"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="base-url">Base URL</Label>
            <Input
              id="base-url"
              value={form.model.base_url}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, base_url: e.target.value } }))}
              placeholder="https://api.minimax.io/v1"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="api-type">API Type</Label>
            <Input
              id="api-type"
              value={form.model.api}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, api: e.target.value } }))}
              placeholder="openai-completions"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="auth-type">Auth Type</Label>
            <Input
              id="auth-type"
              value={form.model.auth}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, auth: e.target.value } }))}
              placeholder="api-key"
              disabled={loading || saving}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="api-key">API Key</Label>
            <Input
              id="api-key"
              type="password"
              value={form.model.api_key}
              onChange={(e) => setForm((prev) => ({ ...prev, model: { ...prev.model, api_key: e.target.value } }))}
              placeholder="输入统一管理的模型 API Key"
              disabled={loading || saving}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">已配置提供商</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {sortedProviders.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂时还没有已配置提供商。</p>
          ) : (
            sortedProviders.map((provider) => (
              <div key={provider.name} className="rounded-lg border p-4 space-y-3">
                <div className="flex flex-col gap-2 md:flex-row md:items-start md:justify-between">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium">{provider.name}</p>
                      {provider.is_default && (
                        <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs text-primary">
                          默认
                        </span>
                      )}
                    </div>
                    <p className="text-sm text-muted-foreground">{provider.id}</p>
                    <p className="text-xs text-muted-foreground break-all">{provider.base_url}</p>
                    <p className="text-xs text-muted-foreground">
                      {provider.api} · {provider.auth}
                    </p>
                  </div>
                  <div className="flex gap-2">
                    {!provider.is_default && (
                      <Button
                        variant="outline"
                        onClick={() => handleUseAsDefault(provider)}
                        disabled={saving}
                      >
                        设为默认
                      </Button>
                    )}
                    <Button
                      variant="outline"
                      onClick={() => handleTestProvider(provider)}
                      disabled={testingName === provider.name}
                    >
                      {testingName === provider.name ? "测试中..." : "一键测试"}
                    </Button>
                  </div>
                </div>
              </div>
            ))
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">新增提供商</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="new-provider-name">名称</Label>
            <Input
              id="new-provider-name"
              value={providerForm.name}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, name: e.target.value }))}
              placeholder="minimax"
              disabled={saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-provider-id">模型 ID</Label>
            <Input
              id="new-provider-id"
              value={providerForm.id}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, id: e.target.value }))}
              placeholder="MiniMax-M2.5-highspeed"
              disabled={saving}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="new-provider-base-url">Base URL</Label>
            <Input
              id="new-provider-base-url"
              value={providerForm.base_url}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, base_url: e.target.value }))}
              placeholder="https://api.minimax.io/v1"
              disabled={saving}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-provider-api">API Type</Label>
            <Select
              value={providerForm.api}
              onValueChange={(value) => {
                if (!value) return;
                setProviderForm((prev) => ({ ...prev, api: value }));
              }}
              disabled={saving}
            >
              <SelectTrigger id="new-provider-api">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="openai-completions">openai-completions</SelectItem>
                <SelectItem value="openai-responses">openai-responses</SelectItem>
                <SelectItem value="anthropic-messages">anthropic-messages</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-provider-auth">Auth Type</Label>
            <Select
              value={providerForm.auth}
              onValueChange={(value) => {
                if (!value) return;
                setProviderForm((prev) => ({ ...prev, auth: value }));
              }}
              disabled={saving}
            >
              <SelectTrigger id="new-provider-auth">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="api-key">api-key</SelectItem>
                <SelectItem value="bearer">bearer</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="new-provider-api-key">API Key</Label>
            <Input
              id="new-provider-api-key"
              type="password"
              value={providerForm.api_key}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, api_key: e.target.value }))}
              placeholder="输入该提供商的 API Key"
              disabled={saving}
            />
          </div>
          <div className="flex items-center gap-2 md:col-span-2">
            <input
              id="set-default-provider"
              type="checkbox"
              checked={providerForm.set_default}
              onChange={(e) => setProviderForm((prev) => ({ ...prev, set_default: e.target.checked }))}
              disabled={saving}
            />
            <Label htmlFor="set-default-provider">保存后设为默认提供商</Label>
          </div>
          <div className="md:col-span-2 flex justify-end">
            <Button onClick={handleSaveProvider} disabled={saving}>
              {saving ? "保存中..." : "新增提供商"}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">Telegram 默认配置</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="telegram-dm-policy">DM Policy</Label>
            <Select
              value={form.telegram.dm_policy}
              onValueChange={(value) => {
                if (!value) return;
                setForm((prev) => ({ ...prev, telegram: { ...prev.telegram, dm_policy: value } }));
              }}
              disabled={loading || saving}
            >
              <SelectTrigger id="telegram-dm-policy">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="open">open</SelectItem>
                <SelectItem value="pairing">pairing</SelectItem>
                <SelectItem value="allowlist">allowlist</SelectItem>
                <SelectItem value="disabled">disabled</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="telegram-group-policy">Group Policy</Label>
            <Select
              value={form.telegram.group_policy}
              onValueChange={(value) => {
                if (!value) return;
                setForm((prev) => ({ ...prev, telegram: { ...prev.telegram, group_policy: value } }));
              }}
              disabled={loading || saving}
            >
              <SelectTrigger id="telegram-group-policy">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="allowlist">allowlist</SelectItem>
                <SelectItem value="open">open</SelectItem>
                <SelectItem value="disabled">disabled</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button onClick={handleSave} disabled={loading || saving}>
          {saving ? "保存中..." : "保存默认配置"}
        </Button>
      </div>
    </div>
  );
}
