import { useEffect, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { Copy, Download, Check } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { getToken } from '@/api/client';
import { peerConfigUrl } from '@/api/peers';
import type { Peer } from '@/types/api';

interface PeerConfigModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  networkId: number;
  peer: Peer | null;
}

export function PeerConfigModal({
  open,
  onOpenChange,
  networkId,
  peer,
}: PeerConfigModalProps) {
  const [config, setConfig] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open || !peer) {
      setConfig(null);
      setCopied(false);
      return;
    }

    const fetchConfig = async () => {
      const token = getToken();
      const response = await fetch(
        peerConfigUrl(networkId, peer.id),
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
        },
      );
      if (response.ok) {
        setConfig(await response.text());
      }
    };
    fetchConfig();
  }, [open, peer, networkId]);

  const handleCopy = async () => {
    if (!config) return;
    await navigator.clipboard.writeText(config);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {peer ? `Config: ${peer.name}` : 'Peer Config'}
          </DialogTitle>
          <DialogDescription>
            Scan the QR code with WireGuard mobile or download the .conf file.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {config ? (
            <>
              <div className="flex justify-center rounded-md border bg-white p-4">
                <QRCodeSVG value={config} size={200} />
              </div>

              <pre className="max-h-48 overflow-auto rounded-md bg-muted p-3 text-xs font-mono">
                {config}
              </pre>

              <div className="flex gap-2">
                <Button
                  variant="outline"
                  className="flex-1"
                  onClick={handleCopy}
                >
                  {copied ? (
                    <Check className="mr-2 h-4 w-4" />
                  ) : (
                    <Copy className="mr-2 h-4 w-4" />
                  )}
                  {copied ? 'Copied' : 'Copy'}
                </Button>
                {peer && (
                  <Button variant="outline" className="flex-1" asChild>
                    <a
                      href={peerConfigUrl(networkId, peer.id)}
                      download={`wgpilot-${peer.name}.conf`}
                    >
                      <Download className="mr-2 h-4 w-4" />
                      Download .conf
                    </a>
                  </Button>
                )}
              </div>
            </>
          ) : (
            <div className="flex h-48 items-center justify-center text-muted-foreground">
              Loading config...
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
