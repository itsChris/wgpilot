import { useState, useEffect } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { Shield, User, Server, Network, Smartphone } from 'lucide-react';
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card';
import { useSetupStatus } from '@/api/setup';
import { StepAdmin } from './step-admin';
import { StepServer } from './step-server';
import { StepNetwork } from './step-network';
import { StepPeer } from './step-peer';

const steps = [
  { label: 'Admin', icon: User },
  { label: 'Server', icon: Server },
  { label: 'Network', icon: Network },
  { label: 'Peer', icon: Smartphone },
] as const;

export function SetupWizard() {
  const navigate = useNavigate();
  const { data: status, isLoading } = useSetupStatus();
  const [currentStep, setCurrentStep] = useState(1);

  // Sync with server-reported step on load.
  useEffect(() => {
    if (status) {
      if (status.complete) {
        navigate({ to: '/' });
        return;
      }
      setCurrentStep(status.current_step);
    }
  }, [status, navigate]);

  const handleStepComplete = () => {
    if (currentStep >= 4) {
      navigate({ to: '/' });
      return;
    }
    setCurrentStep((prev) => prev + 1);
  };

  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="text-muted-foreground">Loading...</div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 py-8">
      <div className="w-full max-w-lg space-y-6">
        {/* Header */}
        <div className="text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Shield className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold">wgpilot Setup</h1>
          <p className="text-sm text-muted-foreground">
            Configure your WireGuard server in a few steps.
          </p>
        </div>

        {/* Progress indicator */}
        <div className="flex items-center justify-center gap-1">
          {steps.map((step, idx) => {
            const stepNum = idx + 1;
            const Icon = step.icon;
            const isActive = stepNum === currentStep;
            const isComplete = stepNum < currentStep;

            return (
              <div key={step.label} className="flex items-center">
                {idx > 0 && (
                  <div
                    className={`mx-1 h-px w-8 ${
                      isComplete ? 'bg-primary' : 'bg-border'
                    }`}
                  />
                )}
                <div
                  className={`flex h-9 w-9 items-center justify-center rounded-full border-2 transition-colors ${
                    isActive
                      ? 'border-primary bg-primary text-primary-foreground'
                      : isComplete
                        ? 'border-primary bg-primary/10 text-primary'
                        : 'border-border text-muted-foreground'
                  }`}
                  title={step.label}
                >
                  <Icon className="h-4 w-4" />
                </div>
              </div>
            );
          })}
        </div>

        {/* Step card */}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Step {currentStep} of 4
            </CardTitle>
            <CardDescription className="sr-only">
              {steps[currentStep - 1]?.label}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {currentStep === 1 && <StepAdmin onComplete={handleStepComplete} />}
            {currentStep === 2 && <StepServer onComplete={handleStepComplete} />}
            {currentStep === 3 && <StepNetwork onComplete={handleStepComplete} />}
            {currentStep === 4 && <StepPeer onComplete={handleStepComplete} />}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
