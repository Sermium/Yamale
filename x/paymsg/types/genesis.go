package types

import "fmt"

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:                    DefaultParams(),
		ParticipantApplicationMap: []ParticipantApplication{}, ApprovedParticipantMap: []ApprovedParticipant{}, PaymentRecordMap: []PaymentRecord{}}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	participantApplicationIndexMap := make(map[string]struct{})

	for _, elem := range gs.ParticipantApplicationMap {
		index := fmt.Sprint(elem.Creator)
		if _, ok := participantApplicationIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for participantApplication")
		}
		participantApplicationIndexMap[index] = struct{}{}
	}
	approvedParticipantIndexMap := make(map[string]struct{})

	for _, elem := range gs.ApprovedParticipantMap {
		index := fmt.Sprint(elem.Participant)
		if _, ok := approvedParticipantIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for approvedParticipant")
		}
		approvedParticipantIndexMap[index] = struct{}{}
	}
	paymentRecordIndexMap := make(map[string]struct{})

	for _, elem := range gs.PaymentRecordMap {
		index := fmt.Sprint(elem.EndToEndId)
		if _, ok := paymentRecordIndexMap[index]; ok {
			return fmt.Errorf("duplicated index for paymentRecord")
		}
		paymentRecordIndexMap[index] = struct{}{}
	}

	return gs.Params.Validate()
}
