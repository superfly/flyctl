package machine

import (
	"context"
	"fmt"
	"slices"
	"time"
	"github.com/superfly/flyctl/internal/prompt"
	"os/exec"
	"os"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/watch"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/maps"
)

var cpusPerKind = map[string][]int{
	"shared":      {1, 2, 4, 6, 8},
	"performance": {1, 2, 4, 6, 8, 10, 12, 14, 16},
}

func Update(ctx context.Context, m *fly.Machine, input *fly.LaunchMachineInput) error {
	var (
		flapsClient    = flapsutil.ClientFromContext(ctx)
		io             = iostreams.FromContext(ctx)
		colorize       = io.ColorScheme()
		updatedMachine *fly.Machine
		err            error
	)

	if input != nil && input.Config != nil && input.Config.Guest != nil {
		var invalidConfigErr InvalidConfigErr
		invalidConfigErr.guest = input.Config.Guest
		invalidConfigErr.ctx = ctx

		// Check that there's a valid number of CPUs
		validNumCpus, ok := cpusPerKind[input.Config.Guest.CPUKind]
		if !ok {
			invalidConfigErr.Reason = invalidCpuKind
			return invalidConfigErr
		} else if !slices.Contains(validNumCpus, input.Config.Guest.CPUs) {
			invalidConfigErr.Reason = invalidNumCPUs
			return invalidConfigErr
		}

		if input.Config.Guest.CPUKind == "shared" && input.Config.Guest.MemoryMB%256 != 0 {
			invalidConfigErr.Reason = invalidMemorySize
			return invalidConfigErr
		} else if input.Config.Guest.CPUKind == "performance" && input.Config.Guest.MemoryMB%1024 != 0 {
			invalidConfigErr.Reason = invalidMemorySize
			return invalidConfigErr
		}

		// Check memory sizes
		var min_memory_size int

		if input.Config.Guest.CPUKind == "shared" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * input.Config.Guest.CPUs
		} else if input.Config.Guest.CPUKind == "performance" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * input.Config.Guest.CPUs
		}

		if min_memory_size > input.Config.Guest.MemoryMB {
			invalidConfigErr.Reason = memoryTooLow
			return invalidConfigErr
		}

		var maxMemory int

		if input.Config.Guest.CPUKind == "shared" {
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_SHARED_CPU
		} else if input.Config.Guest.CPUKind == "performance" {
			maxMemory = input.Config.Guest.CPUs * fly.MAX_MEMORY_MB_PER_CPU
		}

		if input.Config.Guest.MemoryMB > maxMemory {
			invalidConfigErr.Reason = memoryTooHigh
			return invalidConfigErr
		}

	}

	fmt.Fprintf(io.Out, "Updating machine %s\n", colorize.Bold(m.ID))

	input.ID = m.ID
	updatedMachine, err = flapsClient.Update(ctx, *input, m.LeaseNonce)
	if err != nil {
		return fmt.Errorf("could not update machine %s: %w", m.ID, err)
	}

	waitForAction := "start"
	if input.SkipLaunch || m.Config.Schedule != "" {
		waitForAction = "stop"
	}

	waitTimeout := time.Second * 300
	if input.Timeout != 0 {
		waitTimeout = time.Duration(input.Timeout) * time.Second
	}

	if err := WaitForStartOrStop(ctx, updatedMachine, waitForAction, waitTimeout); err != nil {
		return err
	}

	if !input.SkipLaunch {
		if !input.SkipHealthChecks {
			if err := watch.MachinesChecks(ctx, []*fly.Machine{updatedMachine}); err != nil {
				return fmt.Errorf("failed to wait for health checks to pass: %w", err)
			}
		}
	}

	fmt.Fprintf(io.Out, "Machine %s updated successfully!\n", colorize.Bold(m.ID))

	return nil
}

type invalidConfigReason string

const (
	invalidCpuKind    invalidConfigReason = "invalid CPU kind"
	invalidNumCPUs    invalidConfigReason = "invalid number of CPUs"
	invalidMemorySize invalidConfigReason = "invalid memory size"
	memoryTooLow      invalidConfigReason = "memory size for config is too low"
	memoryTooHigh     invalidConfigReason = "memory size for config is too high"
)

type InvalidConfigErr struct {
	Reason invalidConfigReason
	guest  *fly.MachineGuest
	ctx context.Context
}

func (e InvalidConfigErr) Description() string {
	switch e.Reason {
	case invalidCpuKind:
		return fmt.Sprintf("The CPU kind given: %s, is not valid", e.guest.CPUKind)
	case invalidNumCPUs:
		return fmt.Sprintf("For the CPU kind %s, %d CPUs is not valid", e.guest.CPUKind, e.guest.CPUs)
	case invalidMemorySize:
		return fmt.Sprintf("%dMiB of memory is not valid", e.guest.MemoryMB)
	case memoryTooLow:
		return fmt.Sprintf("For %s VMs with %d CPUs, %dMiB of memory is too low", e.guest.CPUKind, e.guest.CPUs, e.guest.MemoryMB)
	case memoryTooHigh:
		return fmt.Sprintf("For %s VMs with %d CPUs, %dMiB of memory is too high", e.guest.CPUKind, e.guest.CPUs, e.guest.MemoryMB)
	}
	return string(e.Reason)
}

func (e InvalidConfigErr) Suggestion() string {
	switch e.Reason {
	case invalidCpuKind:
		return fmt.Sprintf("Valid values are %v", maps.Keys(cpusPerKind))
	case invalidNumCPUs:
		validNumCpus := cpusPerKind[e.guest.CPUKind]
		return fmt.Sprintf("Valid numbers are %v", validNumCpus)
	case invalidMemorySize:
		var incrementSize int = 1024
		switch e.guest.CPUKind {
		case "shared":
			incrementSize = 256
		case "performance":
			incrementSize = 1024
		}

		suggestedSize := e.guest.MemoryMB - (e.guest.MemoryMB % incrementSize)
		if suggestedSize == 0 {
			suggestedSize = incrementSize
		}

		return fmt.Sprintf("Memory size must be in a %d MiB increment (%dMiB would work)", incrementSize, suggestedSize)
	case memoryTooLow:
		var min_memory_size int

		if e.guest.CPUKind == "shared" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		} else if e.guest.CPUKind == "performance" {
			min_memory_size = fly.MIN_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value >= %dMiB for the config changes requested", min_memory_size)

	case memoryTooHigh:
		var max_memory_size int
		if e.guest.CPUKind == "shared" {
			max_memory_size = fly.MAX_MEMORY_MB_PER_SHARED_CPU * e.guest.CPUs
		} else if e.guest.CPUKind == "performance" {
			max_memory_size = fly.MAX_MEMORY_MB_PER_CPU * e.guest.CPUs
		}

		return fmt.Sprintf("Try setting memory to a value <= %dMiB for the config changes requested", max_memory_size)
	}

	return ""
}

func (e InvalidConfigErr) DocURL() string {
	return "https://fly.io/docs/machines/guides-examples/machine-sizing/"
}

func (e InvalidConfigErr) Error() string {
	return string(e.Reason)
}


func (e InvalidConfigErr) AttemptFix() (string,error) {
	unsuccessfull := "Unsuccessful at fixing the error attempt!"
	switch e.Reason {
	case memoryTooHigh:
		// Get correct CPU count
		required_cpu_count,err := getRequiredCPUForMemoryTooHighError(e)
		if err==nil{
			combo := string(fmt.Sprintf("%s-cpu-%dx",e.guest.CPUKind,required_cpu_count))
			// Notify of issue
			//fmt.Println( "\nWarning! "+e.Description()+"! To scale your memory to "+string(fmt.Sprintf( "%d",e.guest.MemoryMB))+"MiB, 
			//you must first increase your CPU cores to "+string(fmt.Sprintf( "%d",required_cpu_count)) +", which can be accomodated by a \""+combo+"\" VM size.\n")
			

			// Prompt
			//If you would like to proceed with scaling your memory to %dMiB, your Machine's CPU count must be increased to %d CPUs.",e.Description(),e.guest.MemoryMB,required_cpu_count
			prompt_str :=  fmt.Sprintf("Warning! %s! A memory of %dMiB requires %d CPU cores, "+
			"which can be accomodated by a \"%s\" VM size.\n Would you like to scale your VM size to %s by running the command `fly scale vm %s`?"+
			"\n Agreeing will update your VM to \"%s\" size first, then proceed with scaling the memory to the requested %dMiB", 
			e.Description(), e.guest.MemoryMB, required_cpu_count, combo, combo, combo,combo, e.guest.MemoryMB)
			//combo, combo, combo, e.guest.MemoryMB )
			yesScaleCPUFirst, no := prompt.Confirm( e.ctx, prompt_str)	

			// Scale CPU
			fmt.Println(yesScaleCPUFirst)
			fmt.Println(no)
			if yesScaleCPUFirst{
				fmt.Println("Running fly scale vm command...")
				flyctl, err := exec.LookPath("fly")
				if err== nil{
					
					// attempt to install bundle before proceeding
					args := []string{"scale", "vm", fmt.Sprintf("%s-cpu-%dx",e.guest.CPUKind,required_cpu_count)}
					fmt.Println(args)
					cmd := exec.Command(flyctl, args...)
					cmd.Stdin = os.Stdin
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
					cmd.Run()

					fmt.Println("success!")

					return "success!",nil
				}
			}

		}else{
			fmt.Println("returning error...")
			return err.Error(), err
		}
		
	}
	fmt.Println("returning last error..")
	return unsuccessfull, fmt.Errorf( unsuccessfull )
}

//func getRequiredVmScaleCommandForMemoryTooHighError(){}
func getRequiredCPUForMemoryTooHighError( e InvalidConfigErr ) (int, error){
	var error_check = ""
	var required_cpu_count = e.guest.CPUs
	for error_check == ""{
		// Increment cpu count to see if that would satisfy the requested memory 
		required_cpu_count = required_cpu_count*2
		//fmt.Println("Checking if %d cpu count works!",required_cpu_count)

		// Validate new cpu count is valid 
		validNumCpus, ok := cpusPerKind[e.guest.CPUKind]
		if !ok {
			fmt.Println("Not valid cpu kind!")
			error_check = string(invalidCpuKind)
		} else if !slices.Contains(validNumCpus, required_cpu_count) {
			fmt.Println("Not valid cpu count for kind!")
			error_check = string(invalidNumCPUs)
		}else{

			// Validate that the new cpu count's max memory can accommodate the requested memory
			var maxMemory int

			// Get the max memory the new cpu count can hold
			if e.guest.CPUKind == "shared" {
				maxMemory = required_cpu_count * fly.MAX_MEMORY_MB_PER_SHARED_CPU
			} else if e.guest.CPUKind == "performance" {
				maxMemory = required_cpu_count * fly.MAX_MEMORY_MB_PER_CPU
			}

			// See if its max memory can hold the requested memory
			if e.guest.MemoryMB <= maxMemory{
				return required_cpu_count, nil
			}
		}
	}
	// Sorry, can't get the cpu count
	general_error := "Unable to retrieve the required CPU count to accommodate requested memory!"
	return required_cpu_count, fmt.Errorf( general_error+"\n\n"+error_check )
}