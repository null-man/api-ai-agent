"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { useAuth } from "@/components/auth-provider";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { getBotDebug, listBots, type Bot, type BotDebugData } from "@/lib/api";

const statusStyles: Record<string, string> = {
  running: "bg-green-100 text-green-700 border-green-200",
  starting: "bg-yellow-100 text-yellow-700 border-yellow-200",
  created: "bg-blue-100 text-blue-700 border-blue-200",
  stopped: "bg-gray-100 text-gray-600 border-gray-200",
  error: "bg-red-100 text-red-700 border-red-200",
};

export default function DebugPage() {
  const { isAuthed } = useAuth();
  const [bots, setBots] = useState<Bot[]>([]);
  const [loadingBots, setLoadingBots] = useState(true);
  const [loadingDebug, setLoadingDebug] = useState(false);
  const [selectedBotId, setSelectedBotId] = useState("");
  const [tail, setTail] = useState("200");
  const [query, setQuery] = useState("");
  const [debugData, setDebugData] = useState<BotDebugData | null>(null);

  const filteredBots = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return bots;
    return bots.filter((bot) =>
      [bot.name, bot.user_id, bot.slug, bot.id].some((value) =>
        value.toLowerCase().includes(needle)
      )
    );
  }, [bots, query]);

  const fetchBots = useCallback(async () => {
    try {
      setLoadingBots(true);
      const res = await listBots();
      const nextBots = res.data || [];
      setBots(nextBots);
      if (!selectedBotId && nextBots.length > 0) {
        setSelectedBotId(nextBots[0].id);
      }
    } catch (err) {
      toast.error("Failed to load bots: " + (err as Error).message);
    } finally {
      setLoadingBots(false);
    }
  }, [selectedBotId]);

  const fetchDebug = useCallback(
    async (botId: string) => {
      if (!botId) return;
      try {
        setLoadingDebug(true);
        const parsedTail = Number.parseInt(tail, 10);
        const effectiveTail = Number.isFinite(parsedTail) && parsedTail > 0 ? parsedTail : 200;
        const res = await getBotDebug(botId, effectiveTail);
        setDebugData(res.data);
      } catch (err) {
        setDebugData(null);
        toast.error("Failed to load debug data: " + (err as Error).message);
      } finally {
        setLoadingDebug(false);
      }
    },
    [tail]
  );

  useEffect(() => {
    if (isAuthed) {
      fetchBots();
    }
  }, [isAuthed, fetchBots]);

  useEffect(() => {
    if (isAuthed && selectedBotId) {
      fetchDebug(selectedBotId);
    }
  }, [isAuthed, selectedBotId, fetchDebug]);

  if (!isAuthed) return null;

  const selectedBot = bots.find((bot) => bot.id === selectedBotId) || null;

  return (
    <div className="p-4 md:p-6 space-y-4 md:space-y-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-xl md:text-2xl font-bold">Debug</h1>
        <p className="text-sm text-muted-foreground">
          View current bot health, latest pod, restart count, and recent runtime logs.
        </p>
      </div>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">Bot Selection</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-[minmax(0,1fr)_160px_auto]">
          <div className="space-y-2">
            <Label htmlFor="bot-search">Search</Label>
            <Input
              id="bot-search"
              placeholder="Search by name, user ID, slug, or bot ID"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="tail-lines">Tail Lines</Label>
            <Input
              id="tail-lines"
              inputMode="numeric"
              value={tail}
              onChange={(e) => setTail(e.target.value)}
            />
          </div>
          <div className="flex items-end">
            <Button onClick={() => fetchDebug(selectedBotId)} disabled={!selectedBotId || loadingDebug}>
              {loadingDebug ? "Refreshing..." : "Refresh"}
            </Button>
          </div>
          <div className="space-y-2 md:col-span-3">
            <Label htmlFor="bot-select">Bot</Label>
            <Select value={selectedBotId} onValueChange={(value) => setSelectedBotId(value ?? "")}>
              <SelectTrigger id="bot-select">
                <SelectValue placeholder={loadingBots ? "Loading bots..." : "Select a bot"} />
              </SelectTrigger>
              <SelectContent>
                {filteredBots.map((bot) => (
                  <SelectItem key={bot.id} value={bot.id}>
                    {bot.name} · {bot.user_id} · {bot.slug}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>

      {selectedBot && (
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(360px,0.9fr)]">
          <div className="space-y-4">
            <Card>
              <CardHeader className="pb-4">
                <CardTitle className="text-base">Bot Status</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3 sm:grid-cols-2">
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Name</p>
                  <p className="font-medium">{selectedBot.name}</p>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Status</p>
                  <Badge variant="outline" className={statusStyles[debugData?.bot.status || selectedBot.status] || ""}>
                    {debugData?.bot.status || selectedBot.status}
                  </Badge>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">User ID</p>
                  <code className="text-xs">{selectedBot.user_id}</code>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Slug</p>
                  <code className="text-xs">{selectedBot.slug}</code>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Bot ID</p>
                  <code className="text-xs break-all">{selectedBot.id}</code>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Endpoint</p>
                  <code className="text-xs break-all">{debugData?.bot.endpoint || selectedBot.endpoint || "n/a"}</code>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-4">
                <CardTitle className="text-base">Deployment</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3 sm:grid-cols-2">
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Deployment Status</p>
                  <p className="font-medium">{debugData?.bot.deployment_status?.status || "n/a"}</p>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Ready Replicas</p>
                  <p className="font-medium">
                    {debugData?.bot.deployment_status?.ready_replicas ?? 0}/
                    {debugData?.bot.deployment_status?.desired_replicas ?? 0}
                  </p>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Image</p>
                  <code className="text-xs break-all">{debugData?.bot.image || "n/a"}</code>
                </div>
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Latest Image</p>
                  <code className="text-xs break-all">{debugData?.bot.latest_image || "n/a"}</code>
                </div>
              </CardContent>
            </Card>
          </div>

          <div className="space-y-4">
            <Card>
              <CardHeader className="pb-4">
                <CardTitle className="text-base">Latest Pod</CardTitle>
              </CardHeader>
              <CardContent className="grid gap-3">
                <div>
                  <p className="text-xs uppercase text-muted-foreground">Pod Name</p>
                  <code className="text-xs break-all">{debugData?.pod?.name || "n/a"}</code>
                </div>
                <div className="grid grid-cols-3 gap-3">
                  <div>
                    <p className="text-xs uppercase text-muted-foreground">Phase</p>
                    <p className="font-medium">{debugData?.pod?.phase || "n/a"}</p>
                  </div>
                  <div>
                    <p className="text-xs uppercase text-muted-foreground">Ready</p>
                    <p className="font-medium">{debugData?.pod ? (debugData.pod.ready ? "Yes" : "No") : "n/a"}</p>
                  </div>
                  <div>
                    <p className="text-xs uppercase text-muted-foreground">Restarts</p>
                    <p className="font-medium">{debugData?.pod?.restart_count ?? "n/a"}</p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-base">Recent Logs</CardTitle>
        </CardHeader>
        <CardContent>
          <Textarea
            value={debugData?.logs || ""}
            readOnly
            className="min-h-[420px] resize-y font-mono text-xs leading-5"
            placeholder={loadingDebug ? "Loading logs..." : "No logs available yet."}
          />
        </CardContent>
      </Card>
    </div>
  );
}
