package resourceprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilypad/pkg/http"
	"github.com/bacalhau-project/lilypad/pkg/solver"
	"github.com/bacalhau-project/lilypad/pkg/solver/store"
	"github.com/bacalhau-project/lilypad/pkg/system"
	"github.com/bacalhau-project/lilypad/pkg/web3"
	"github.com/bacalhau-project/lilypad/pkg/web3/bindings/storage"
)

type ResourceProviderController struct {
	solverClient *solver.SolverClient
	options      ResourceProviderOptions
	web3SDK      *web3.Web3SDK
	web3Events   *web3.EventChannels
}

func NewResourceProviderController(
	options ResourceProviderOptions,
	web3SDK *web3.Web3SDK,
) (*ResourceProviderController, error) {
	// we know the address of the solver but what is it's url?
	solverUrl, err := web3SDK.GetSolverUrl(options.Web3.SolverAddress)
	if err != nil {
		return nil, err
	}

	solverClient, err := solver.NewSolverClient(http.ClientOptions{
		URL:        solverUrl,
		PrivateKey: options.Web3.PrivateKey,
	})
	if err != nil {
		return nil, err
	}

	controller := &ResourceProviderController{
		solverClient: solverClient,
		options:      options,
		web3SDK:      web3SDK,
		web3Events:   web3.NewEventChannels(),
	}
	return controller, nil
}

/*
*
*
*

	Setup

*
*
*
*/
func (controller *ResourceProviderController) subscribeToSolver() error {
	controller.solverClient.SubscribeEvents(func(ev solver.SolverEvent) {
		solver.ServiceLogSolverEvent(system.ResourceProviderService, ev)
		// we need to agree to the deal now we've heard about it
		if ev.EventType == solver.DealAdded {
			if ev.Deal == nil {
				system.Error(system.ResourceProviderService, "solver event", fmt.Errorf("RP received nil deal"))
				return
			}

			// check if this deal is for us
			if ev.Deal.ResourceProvider != controller.web3SDK.GetAddress().String() {
				return
			}
		}
	})
	return nil
}

func (controller *ResourceProviderController) subscribeToWeb3() error {
	controller.web3Events.Storage.SubscribeDealStateChange(func(ev storage.StorageDealStateChange) {
		system.Info(system.ResourceProviderService, "StorageDealStateChange", ev)
	})
	return nil
}

func (controller *ResourceProviderController) Start(ctx context.Context, cm *system.CleanupManager) chan error {
	errorChan := make(chan error)
	err := controller.subscribeToSolver()
	if err != nil {
		errorChan <- err
		return errorChan
	}
	err = controller.subscribeToWeb3()
	if err != nil {
		errorChan <- err
		return errorChan
	}
	err = controller.solverClient.Start(ctx, cm)
	if err != nil {
		errorChan <- err
		return errorChan
	}
	err = controller.web3Events.Start(controller.web3SDK, ctx, cm)
	if err != nil {
		errorChan <- err
		return errorChan
	}

	go func() {
		for {
			err := controller.solve()
			if err != nil {
				system.Error(system.ResourceProviderService, "error solving", err)
				errorChan <- err
				return
			}
			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
	return errorChan
}

/*
 *
 *
 *

 Solve

 *
 *
 *
*/

func (controller *ResourceProviderController) solve() error {
	system.Debug(system.ResourceProviderService, "solving", "")
	err := controller.agreeToDeals()
	if err != nil {
		return err
	}
	err = controller.ensureResourceOffers()
	if err != nil {
		return err
	}
	return nil
}

/*
 *
 *
 *

 Agree to deals

 *
 *
 *
*/

// list the deals we have been assigned to that we have not yet posted to the contract
func (controller *ResourceProviderController) agreeToDeals() error {
	// load the deals that are in DealNegotiating
	// and do not have a TransactionsResourceProvider.Agree tx
	negotiatingDeals, err := controller.solverClient.GetDeals(store.GetDealsQuery{
		ResourceProvider: controller.web3SDK.GetAddress().String(),
		State:            "DealNegotiating",
	})
	if err != nil {
		return err
	}
	if len(negotiatingDeals) <= 0 {
		return nil
	}

	// map over the deals and agree to them
	for _, deal := range negotiatingDeals {
		system.Info(system.ResourceProviderService, "agree to deal", deal)

		// tx, err := controller.web3SDK.Contracts.Controller.Agree()
	}

	return err
}

/*
 *
 *
 *

 Ensure resource offers

 *
 *
 *
*/

/*
Ensure resource offers are posted to the solve
*/

func (controller *ResourceProviderController) getResourceOffer(index int, spec data.MachineSpec) data.ResourceOffer {
	return data.ResourceOffer{
		// assign CreatedAt to the current millisecond timestamp
		CreatedAt:        int(time.Now().UnixNano() / int64(time.Millisecond)),
		ResourceProvider: controller.web3SDK.GetAddress().String(),
		Index:            index,
		Spec:             spec,
		Modules:          controller.options.Offers.Modules,
		Mode:             controller.options.Offers.Mode,
		DefaultPricing:   controller.options.Offers.DefaultPricing,
		DefaultTimeouts:  controller.options.Offers.DefaultTimeouts,
		ModulePricing:    map[string]data.DealPricing{},
		ModuleTimeouts:   map[string]data.DealTimeouts{},
		TrustedParties:   controller.options.Offers.TrustedParties,
	}
}

func (controller *ResourceProviderController) ensureResourceOffers() error {
	// load the resource offers that are currently active and so should not be replaced
	activeResourceOffers, err := controller.solverClient.GetResourceOffers(store.GetResourceOffersQuery{
		ResourceProvider: controller.web3SDK.GetAddress().String(),
		Active:           true,
	})
	if err != nil {
		return err
	}

	// create a map of the ids of resource offers we have
	// this will allow us to check if we need to create a new one
	// or update an existing one - we use the "index" because
	// the id's are changing because of the timestamps
	existingResourceOffersMap := map[int]data.ResourceOfferContainer{}
	for _, existingResourceOffer := range activeResourceOffers {
		existingResourceOffersMap[existingResourceOffer.ResourceOffer.Index] = existingResourceOffer
	}

	addResourceOffers := []data.ResourceOffer{}

	// map over the specs we have in the config
	for index, spec := range controller.options.Offers.Specs {

		// check if the resource offer already exists
		// if it does then we need to update it
		// if it doesn't then we need to add it
		_, ok := existingResourceOffersMap[index]
		if !ok {
			addResourceOffers = append(addResourceOffers, controller.getResourceOffer(index, spec))
		}
	}

	// add the resource offers we need to add
	for _, resourceOffer := range addResourceOffers {
		system.Info(system.ResourceProviderService, "add resource offer", resourceOffer)
		_, err := controller.solverClient.AddResourceOffer(resourceOffer)
		if err != nil {
			return err
		}
	}

	return err
}
